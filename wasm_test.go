package wazero

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/wasm"
)

func TestRuntime_DecodeModule(t *testing.T) {
	tests := []struct {
		name         string
		source       []byte
		expectedName string
	}{
		{
			name:   "text - no name",
			source: []byte(`(module)`),
		},
		{
			name:   "text - empty name",
			source: []byte(`(module $)`),
		},
		{
			name:         "text - name",
			source:       []byte(`(module $test)`),
			expectedName: "test",
		},
		{
			name:   "binary - no name section",
			source: binary.EncodeModule(&internalwasm.Module{}),
		},
		{
			name:   "binary - empty NameSection.ModuleName",
			source: binary.EncodeModule(&internalwasm.Module{NameSection: &internalwasm.NameSection{}}),
		},
		{
			name:         "binary - NameSection.ModuleName",
			source:       binary.EncodeModule(&internalwasm.Module{NameSection: &internalwasm.NameSection{ModuleName: "test"}}),
			expectedName: "test",
		},
	}

	r := NewRuntime()
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			decoded, err := r.DecodeModule(tc.source)
			require.NoError(t, err)
			require.Equal(t, tc.expectedName, decoded.name)
		})
	}
}

func TestRuntime_DecodeModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		source      []byte
		expectedErr string
	}{
		{
			name:        "nil",
			expectedErr: "source == nil",
		},
		{
			name:        "invalid binary",
			source:      append(binary.Magic, []byte("yolo")...),
			expectedErr: "invalid version header",
		},
		{
			name:        "invalid text",
			source:      []byte(`(modular)`),
			expectedErr: "1:2: unexpected field: modular",
		},
	}

	r := NewRuntime()
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := r.DecodeModule(tc.source)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// TestDecodedModule_WithName tests that we can pre-validate (cache) a module and instantiate it under different
// names. This pattern is used in wapc-go.
func TestDecodedModule_WithName(t *testing.T) {
	r := NewRuntime()
	base, err := r.DecodeModule([]byte(`(module $0 (memory 1))`))
	require.NoError(t, err)

	require.Equal(t, "0", base.name)

	// Use the same runtime to instantiate multiple modules
	internal := r.(*runtime).store
	m1, err := r.NewModule(base.WithName("1"))
	require.NoError(t, err)
	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("1"), m1.(*internalwasm.PublicModule))

	m2, err := r.NewModule(base.WithName("2"))
	require.NoError(t, err)
	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("2"), m2.(*internalwasm.PublicModule))
}

// TestModule_Memory only covers a couple cases to avoid duplication of internal/wasm/runtime_test.go
func TestModule_Memory(t *testing.T) {
	tests := []struct {
		name, wat   string
		expected    bool
		expectedLen uint32
	}{
		{
			name: "no memory",
			wat:  `(module)`,
		},
		{
			name:        "memory exported, one page",
			wat:         `(module (memory $mem 1) (export "memory" (memory $mem)))`,
			expected:    true,
			expectedLen: 65536,
		},
	}

	for _, tt := range tests {
		tc := tt

		r := NewRuntime()
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := r.DecodeModule([]byte(tc.wat))
			require.NoError(t, err)

			// Instantiate the module and get the export of the above hostFn
			module, err := r.NewModule(decoded)
			require.NoError(t, err)

			mem := module.Memory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

// TestModule_Global only covers a couple cases to avoid duplication of internal/wasm/global_test.go
func TestModule_Global(t *testing.T) {
	tests := []struct {
		name                      string
		module                    *internalwasm.Module // module as wat doesn't yet support globals
		expected, expectedMutable bool
	}{
		{
			name:   "no global",
			module: &internalwasm.Module{},
		},
		{
			name: "global not exported",
			module: &internalwasm.Module{
				GlobalSection: []*internalwasm.Global{
					{
						Type: &internalwasm.GlobalType{ValType: internalwasm.ValueTypeI64, Mutable: true},
						Init: &internalwasm.ConstantExpression{Opcode: internalwasm.OpcodeI64Const, Data: []byte{1}},
					},
				},
			},
		},
		{
			name: "global exported",
			module: &internalwasm.Module{
				GlobalSection: []*internalwasm.Global{
					{
						Type: &internalwasm.GlobalType{ValType: internalwasm.ValueTypeI64},
						Init: &internalwasm.ConstantExpression{Opcode: internalwasm.OpcodeI64Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*internalwasm.Export{
					"global": {Type: internalwasm.ExternTypeGlobal, Name: "global"},
				},
			},
			expected: true,
		},
		{
			name: "global exported and mutable",
			module: &internalwasm.Module{
				GlobalSection: []*internalwasm.Global{
					{
						Type: &internalwasm.GlobalType{ValType: internalwasm.ValueTypeI64, Mutable: true},
						Init: &internalwasm.ConstantExpression{Opcode: internalwasm.OpcodeI64Const, Data: []byte{1}},
					},
				},
				ExportSection: map[string]*internalwasm.Export{
					"global": {Type: internalwasm.ExternTypeGlobal, Name: "global"},
				},
			},
			expected:        true,
			expectedMutable: true,
		},
	}

	for _, tt := range tests {
		tc := tt

		r := NewRuntime()
		t.Run(tc.name, func(t *testing.T) {
			// Instantiate the module and get the export of the above global
			module, err := r.NewModule(&DecodedModule{module: tc.module})
			require.NoError(t, err)

			global := module.Global("global")
			if !tc.expected {
				require.Nil(t, global)
				return
			}
			require.Equal(t, uint64(1), global.Get())

			mutable, ok := global.(wasm.MutableGlobal)
			require.Equal(t, tc.expectedMutable, ok)
			if ok {
				mutable.Set(2)
				require.Equal(t, uint64(2), global.Get())
			}
		})
	}
}

func TestFunction_Context(t *testing.T) {
	type key string
	runtimeCtx := context.WithValue(context.Background(), key("wa"), "zero")
	config := NewRuntimeConfig().WithContext(runtimeCtx)

	notStoreCtx := context.WithValue(context.Background(), key("wazer"), "o")

	tests := []struct {
		name     string
		ctx      context.Context
		expected context.Context
	}{
		{
			name:     "nil defaults to runtime context",
			ctx:      nil,
			expected: runtimeCtx,
		},
		{
			name:     "set overrides runtime context",
			ctx:      notStoreCtx,
			expected: notStoreCtx,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntimeWithConfig(config)

			// Define a host function so that we can catch the context propagated from a module function call
			functionName := "fn"
			expectedResult := uint64(math.MaxUint64)
			hostFn := func(ctx wasm.ModuleContext) uint64 {
				require.Equal(t, tc.expected, ctx.Context())
				return expectedResult
			}
			source := requireImportAndExportFunction(t, r, hostFn, functionName)

			// Instantiate the module and get the export of the above hostFn
			decoded, err := r.DecodeModule(source)
			require.NoError(t, err)

			module, err := r.NewModule(decoded)
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := module.Function(functionName).Call(tc.ctx)
			require.NoError(t, err)
			require.Equal(t, expectedResult, results[0])
		})
	}
}

func TestRuntime_NewModule_UsesStoreContext(t *testing.T) {
	type key string
	runtimeCtx := context.WithValue(context.Background(), key("wa"), "zero")
	config := NewRuntimeConfig().WithContext(runtimeCtx)
	r := NewRuntimeWithConfig(config)

	// Define a function that will be set as the start function
	var calledStart bool
	start := func(ctx wasm.ModuleContext) {
		calledStart = true
		require.Equal(t, runtimeCtx, ctx.Context())
	}

	_, err := r.NewHostModule(&HostModuleConfig{Functions: map[string]interface{}{"start": start}})
	require.NoError(t, err)

	decoded, err := r.DecodeModule([]byte(`(module $runtime_test.go
	(import "" "start" (func $start))
	(start $start)
)`))
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	_, err = r.NewModule(decoded)
	require.NoError(t, err)
	require.True(t, calledStart)
}

// requireImportAndExportFunction re-module a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, r Runtime, hostFn func(ctx wasm.ModuleContext) uint64, functionName string) []byte {
	_, err := r.NewHostModule(&HostModuleConfig{
		Name: "host", Functions: map[string]interface{}{functionName: hostFn},
	})
	require.NoError(t, err)

	return []byte(fmt.Sprintf(
		`(module (import "host" "%[1]s" (func (result i64))) (export "%[1]s" (func 0)))`, functionName,
	))
}
