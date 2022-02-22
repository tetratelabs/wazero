package wazero

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

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
			mod, err := DecodeModuleText([]byte(tc.wat))
			require.NoError(t, err)

			// Instantiate the module and get the export of the above hostFn
			exports, err := InstantiateModule(NewStore(), mod)
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
			mod := requireImportAndExportFunction(t, store, hostFn, functionName)

			// Instantiate the module and get the export of the above hostFn
			exports, err := InstantiateModule(store, mod)
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := exports.Function(functionName)(tc.ctx)
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
	_, err := ExportHostFunctions(store, "", map[string]interface{}{"start": start})
	require.NoError(t, err)

	mod, err := DecodeModuleText([]byte(`(module $store_test.go
	(import "" "start" (func $start))
	(start $start)
)`))
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	_, err = InstantiateModule(store, mod)
	require.NoError(t, err)
	require.True(t, calledStart)
}

// requireImportAndExportFunction re-exports a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, store wasm.Store, hostFn func(ctx wasm.ModuleContext) uint64, functionName string) *Module {
	_, err := ExportHostFunctions(store, "host", map[string]interface{}{functionName: hostFn})
	require.NoError(t, err)
	wat := fmt.Sprintf(`(module (import "host" "%[1]s" (func (result i64))) (export "%[1]s" (func 0)))`, functionName)
	mod, err := DecodeModuleText([]byte(wat))
	require.NoError(t, err)
	return mod
}
