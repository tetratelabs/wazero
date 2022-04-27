package bulk_memory_operations

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestBulkMemoryOperations_JIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	testBulkMemoryOperations(t, wazero.NewRuntimeConfigJIT)
}

func TestBulkMemoryOperations_Interpreter(t *testing.T) {
	testBulkMemoryOperations(t, wazero.NewRuntimeConfigInterpreter)
}

// bulkMemoryOperationsWasm was compiled from testdata/bulk_memory_operations.wat
//go:embed testdata/bulk_memory_operations.wasm
var bulkMemoryOperationsWasm []byte

func testBulkMemoryOperations(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("disabled", func(t *testing.T) {
		// bulk-memory-operations is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(testCtx, bulkMemoryOperationsWasm)
		require.Error(t, err)
	})
	t.Run("enabled", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureBulkMemoryOperations(true))

		// Test the example logic.
		t.Run("base case", func(t *testing.T) {
			a, err := r.NewModuleBuilder("a").
				ExportGlobalI32("global", 0). // global=1 ignores the "memory.init" on .start.
				Instantiate(testCtx)
			require.NoError(t, err)
			defer a.Close(testCtx)

			mod, err := r.InstantiateModuleFromCode(testCtx, bulkMemoryOperationsWasm)
			require.NoError(t, err)
			defer mod.Close(testCtx)

			// The first segment is active, so we expect it to be readable.
			bytes, ok := mod.Memory().Read(testCtx, 0, 5)
			require.True(t, ok)
			require.Equal(t, "hello", string(bytes))

			// As the value of the global was zero, we don't expect "memory.init" to have copied the passive segment.
			bytes, ok = mod.Memory().Read(testCtx, 16, 7)
			require.True(t, ok)
			require.Equal(t, make([]byte, 7), bytes)
		})
		t.Run("memory.init", func(t *testing.T) {
			a, err := r.NewModuleBuilder("a").
				ExportGlobalI32("global", 1). // global=1 triggers the "memory.init" on .start.
				Instantiate(testCtx)
			require.NoError(t, err)
			defer a.Close(testCtx)

			mod, err := r.InstantiateModuleFromCode(testCtx, bulkMemoryOperationsWasm)
			require.NoError(t, err)
			defer mod.Close(testCtx)

			// The first segment is active, so we expect it to be readable.
			bytes, ok := mod.Memory().Read(testCtx, 0, 5)
			require.True(t, ok)
			require.Equal(t, "hello", string(bytes))

			// As the value of the global was one, we expect "memory.init" to have copied the passive segment.
			bytes, ok = mod.Memory().Read(testCtx, 16, 7)
			require.True(t, ok)
			require.Equal(t, "goodbye", string(bytes))
		})
	})
}
