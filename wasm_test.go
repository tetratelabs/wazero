package wazero

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/api"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
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
			binary, err := r.CompileModule(tc.source)
			require.NoError(t, err)
			if tc.expectedName != "" {
				require.Equal(t, tc.expectedName, binary.module.NameSection.ModuleName)
			}
		})
	}
}

func TestRuntime_DecodeModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		runtime     Runtime
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
		{
			name:        "RuntimeConfig.memoryMaxPage too large",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryMaxPages(math.MaxUint32)),
			source:      []byte(`(module)`),
			expectedErr: "memoryMaxPages 4294967295 (3 Ti) > specification max 65536 (4 Gi)",
		},
		{
			name:        "memory has too many pages - text",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryMaxPages(2)),
			source:      []byte(`(module (memory 3))`),
			expectedErr: "1:17: min 3 pages (192 Ki) outside range of 2 pages (128 Ki) in module.memory[0]",
		},
		{
			name:        "memory has too many pages - binary",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryMaxPages(2)),
			source:      binary.EncodeModule(&internalwasm.Module{MemorySection: &internalwasm.Memory{Min: 2, Max: 3}}),
			expectedErr: "section memory: max 3 pages (192 Ki) outside range of 2 pages (128 Ki)",
		},
	}

	r := NewRuntime()
	for _, tt := range tests {
		tc := tt

		if tc.runtime == nil {
			tc.runtime = r
		}

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.runtime.CompileModule(tc.source)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
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
			binary, err := r.CompileModule([]byte(tc.wat))
			require.NoError(t, err)

			// Instantiate the module and get the export of the above hostFn
			module, err := r.InstantiateModule(binary)
			require.NoError(t, err)

			mem := module.ExportedMemory("memory")
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
			module, err := r.InstantiateModule(&Binary{module: tc.module})
			require.NoError(t, err)

			global := module.ExportedGlobal("global")
			if !tc.expected {
				require.Nil(t, global)
				return
			}
			require.Equal(t, uint64(1), global.Get())

			mutable, ok := global.(api.MutableGlobal)
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
			hostFn := func(ctx api.Module) uint64 {
				require.Equal(t, tc.expected, ctx.Context())
				return expectedResult
			}
			source, closer := requireImportAndExportFunction(t, r, hostFn, functionName)
			defer closer() // nolint

			// Instantiate the module and get the export of the above hostFn
			binary, err := r.CompileModule(source)
			require.NoError(t, err)

			module, err := r.InstantiateModuleWithConfig(binary, NewModuleConfig().WithName(t.Name()))
			require.NoError(t, err)
			defer module.Close()

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := module.ExportedFunction(functionName).Call(module.WithContext(tc.ctx))
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
	start := func(ctx api.Module) {
		calledStart = true
		require.Equal(t, runtimeCtx, ctx.Context())
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("start", start).Instantiate()
	require.NoError(t, err)

	binary, err := r.CompileModule([]byte(`(module $runtime_test.go
	(import "env" "start" (func $start))
	(start $start)
)`))
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	_, err = r.InstantiateModule(binary)
	require.NoError(t, err)
	require.True(t, calledStart)
}

// TestInstantiateModuleFromCode_DoesntEnforce_Start ensures wapc-go work when modules import WASI, but don't export "_start".
func TestInstantiateModuleFromCode_DoesntEnforce_Start(t *testing.T) {
	r := NewRuntime()

	wasi, err := r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	mod, err := r.InstantiateModuleFromCode([]byte(`(module $wasi_test.go
	(memory 1)
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)
	require.NoError(t, mod.Close())
}

func TestInstantiateModuleFromCode_UsesRuntimeContext(t *testing.T) {
	type key string
	config := NewRuntimeConfig().WithContext(context.WithValue(context.Background(), key("wa"), "zero"))
	r := NewRuntimeWithConfig(config)

	// Define a function that will be re-exported as the WASI function: _start
	var calledStart bool
	start := func(ctx api.Module) {
		calledStart = true
		require.Equal(t, config.ctx, ctx.Context())
	}

	host, err := r.NewModuleBuilder("").ExportFunction("start", start).Instantiate()
	require.NoError(t, err)
	defer host.Close()

	wasi, err := r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	// Start the module as a WASI command. This will fail if the context wasn't as intended.
	mod, err := r.InstantiateModuleFromCode([]byte(`(module $wasi_test.go
	(import "" "start" (func $start))
	(memory 1)
	(export "_start" (func $start))
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)
	defer mod.Close()

	require.True(t, calledStart)
}

// TestInstantiateModuleWithConfig_WithName tests that we can pre-validate (cache) a module and instantiate it under
// different names. This pattern is used in wapc-go.
func TestInstantiateModuleWithConfig_WithName(t *testing.T) {
	r := NewRuntime()
	base, err := r.CompileModule([]byte(`(module $0 (memory 1))`))
	require.NoError(t, err)

	require.Equal(t, "0", base.module.NameSection.ModuleName)

	// Use the same runtime to instantiate multiple modules
	internal := r.(*runtime).store
	m1, err := r.InstantiateModuleWithConfig(base, NewModuleConfig().WithName("1"))
	require.NoError(t, err)
	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("1"), m1)

	m2, err := r.InstantiateModuleWithConfig(base, NewModuleConfig().WithName("2"))
	require.NoError(t, err)
	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("2"), m2)
}

// wasiArg was compiled from examples/testdata/wasi_arg.wat
//go:embed examples/testdata/wasi_arg.wasm
var wasiArg []byte

func TestInstantiateModuleWithConfig(t *testing.T) {
	r := NewRuntime()

	stdout := bytes.NewBuffer(nil)

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := NewModuleConfig().WithStdout(stdout)
	wasi, err := r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	binary, err := r.CompileModule(wasiArg)
	require.NoError(t, err)

	// Re-use the same module many times.
	for _, tc := range []string{"a", "b", "c"} {
		mod, err := r.InstantiateModuleWithConfig(binary, sys.WithArgs(tc).WithName(tc))
		require.NoError(t, err)

		// Ensure the scoped configuration applied. As the args are null-terminated, we append zero (NUL).
		require.Equal(t, append([]byte(tc), 0), stdout.Bytes())

		stdout.Reset()
		require.NoError(t, mod.Close())
	}
}

// requireImportAndExportFunction re-exports a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, r Runtime, hostFn func(ctx api.Module) uint64, functionName string) ([]byte, func() error) {
	mod, err := r.NewModuleBuilder("host").ExportFunction(functionName, hostFn).Instantiate()
	require.NoError(t, err)

	return []byte(fmt.Sprintf(
		`(module (import "host" "%[1]s" (func (result i64))) (export "%[1]s" (func 0)))`, functionName,
	)), mod.Close
}
