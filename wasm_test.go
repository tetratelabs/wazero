package wazero

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasm"
)

func TestDecodeModule(t *testing.T) {
	wat := []byte(`(module $test)`)
	m, err := text.DecodeModule(wat)
	require.NoError(t, err)
	wasm := binary.EncodeModule(m)

	tests := []struct {
		name       string
		moduleName string
		source     []byte
	}{
		{name: "binary", source: wasm},
		{name: "binary - override name", source: wasm, moduleName: "override"},
		{name: "text", source: wat},
		{name: "text - override name", source: wat, moduleName: "override"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			config := &ModuleConfig{Name: tc.moduleName, Source: tc.source}
			_, err := decodeModule(config)
			require.NoError(t, err)
			require.Equal(t, tc.moduleName, config.Name)

			// Avoid adding another test just to check Validate works
			require.NoError(t, config.Validate())
		})
	}

	t.Run("caches repetitive decodes", func(t *testing.T) {
		config := &ModuleConfig{Source: wat}
		m, err := decodeModule(config)
		require.NoError(t, err)

		again, err := decodeModule(config)
		require.NoError(t, err)
		require.Same(t, m, again)

		// Ensure config that only changes the name doesn't have to re-decode the source.
		cloned, err := decodeModule(config.WithName("wazero"))
		require.NoError(t, err)
		require.Same(t, m, cloned)
	})

	t.Run("changing source invalidates decode cache", func(t *testing.T) {
		config := &ModuleConfig{Source: wat}
		m, err := decodeModule(config)
		require.NoError(t, err)

		clonedConfig := config.WithName("wazero")

		// When the source is changed, the module needs to be decoded again
		config.Source = wasm
		again, err := decodeModule(config)
		require.NoError(t, err)
		require.Equal(t, m, again)
		require.NotSame(t, m, again)

		// Any copies of the config shouldn't be invalidated
		cloned, err := decodeModule(clonedConfig)
		require.NoError(t, err)
		require.Same(t, m, cloned)
	})
}

func TestDecodeModule_Errors(t *testing.T) {
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

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			config := &ModuleConfig{Source: tc.source}
			_, err := decodeModule(config)
			require.EqualError(t, err, tc.expectedErr)

			// Avoid adding another test just to check Validate works
			require.EqualError(t, config.Validate(), tc.expectedErr)
		})
	}
}

// TestInstantiateModule_Renamed tests that we can pre-validate (cache) a module and instantiate it under different
// names. This pattern is used in wapc-go.
func TestInstantiateModule_Renamed(t *testing.T) {
	base := &ModuleConfig{Source: []byte(`(module (memory 1))`)}
	require.NoError(t, base.Validate()) // cache

	// Use the same store to instantiate multiple modules
	store := NewStore()

	_, err := InstantiateModule(store, base.WithName("1"))
	require.NoError(t, err)

	_, err = InstantiateModule(store, base.WithName("2"))
	require.NoError(t, err)
}

// TestModuleExports_Memory only covers a couple cases to avoid duplication of internal/wasm/store_test.go
func TestModuleExports_Memory(t *testing.T) {
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

		t.Run(tc.name, func(t *testing.T) {
			// Instantiate the module and get the export of the above hostFn
			exports, err := InstantiateModule(NewStore(), &ModuleConfig{Source: []byte(tc.wat)})
			require.NoError(t, err)

			mem := exports.Memory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

func TestFunction_Context(t *testing.T) {
	type key string
	storeCtx := context.WithValue(context.Background(), key("wa"), "zero")
	config := &StoreConfig{Context: storeCtx}

	notStoreCtx := context.WithValue(context.Background(), key("wazer"), "o")

	tests := []struct {
		name     string
		ctx      context.Context
		expected context.Context
	}{
		{
			name:     "nil defaults to store context",
			ctx:      nil,
			expected: storeCtx,
		},
		{
			name:     "set overrides store context",
			ctx:      notStoreCtx,
			expected: notStoreCtx,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			store := NewStoreWithConfig(config)

			// Define a host function so that we can catch the context propagated from a module function call
			functionName := "fn"
			expectedResult := uint64(math.MaxUint64)
			hostFn := func(ctx wasm.ModuleContext) uint64 {
				require.Equal(t, tc.expected, ctx.Context())
				return expectedResult
			}
			source := requireImportAndExportFunction(t, store, hostFn, functionName)

			// Instantiate the module and get the export of the above hostFn
			exports, err := InstantiateModule(store, &ModuleConfig{Source: source})
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := exports.Function(functionName).Call(tc.ctx)
			require.NoError(t, err)
			require.Equal(t, expectedResult, results[0])
		})
	}
}

func TestInstantiateModule_UsesStoreContext(t *testing.T) {
	type key string
	config := &StoreConfig{Context: context.WithValue(context.Background(), key("wa"), "zero")}
	store := NewStoreWithConfig(config)

	// Define a function that will be set as the start function
	var calledStart bool
	start := func(ctx wasm.ModuleContext) {
		calledStart = true
		require.Equal(t, config.Context, ctx.Context())
	}

	_, err := InstantiateHostModule(store, &HostModuleConfig{Functions: map[string]interface{}{"start": start}})
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	_, err = InstantiateModule(store, &ModuleConfig{Source: []byte(`(module $store_test.go
	(import "" "start" (func $start))
	(start $start)
)`)})
	require.NoError(t, err)
	require.True(t, calledStart)
}

// requireImportAndExportFunction re-exports a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, store wasm.Store, hostFn func(ctx wasm.ModuleContext) uint64, functionName string) []byte {
	_, err := InstantiateHostModule(store, &HostModuleConfig{
		Name: "host", Functions: map[string]interface{}{functionName: hostFn},
	})
	require.NoError(t, err)

	return []byte(fmt.Sprintf(
		`(module (import "host" "%[1]s" (func (result i64))) (export "%[1]s" (func 0)))`, functionName,
	))
}

func TestGetModuleName(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		module     *internalwasm.Module
		expected   string
	}{
		{
			name:   "empty on nil name section",
			module: &internalwasm.Module{},
		},
		{
			name:   "empty on empty NameSection.ModuleName",
			module: &internalwasm.Module{NameSection: &internalwasm.NameSection{}},
		},
		{
			name:     "NameSection.ModuleName",
			module:   &internalwasm.Module{NameSection: &internalwasm.NameSection{ModuleName: "test"}},
			expected: "test",
		},
		{
			name:       "overrides nil name section",
			moduleName: "wazero",
			module:     &internalwasm.Module{},
			expected:   "wazero",
		},
		{
			name:       "overrides empty NameSection.ModuleName",
			moduleName: "wazero",
			module:     &internalwasm.Module{NameSection: &internalwasm.NameSection{}},
			expected:   "wazero",
		},
		{
			name:       "overrides NameSection.ModuleName",
			moduleName: "wazero",
			module:     &internalwasm.Module{NameSection: &internalwasm.NameSection{ModuleName: "test"}},
			expected:   "wazero",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, getModuleName(tc.moduleName, tc.module))
		})
	}
}
