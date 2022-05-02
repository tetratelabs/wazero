package bulk_memory_operations

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestBulkMemoryOperations_JIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	testBulkMemoryOperations(t, wazero.NewRuntimeConfigJIT)
	testTableCopy(t, wazero.NewRuntimeConfigJIT)
	testTableInit(t, wazero.NewRuntimeConfigJIT)
	testElemDrop(t, wazero.NewRuntimeConfigJIT)
}

func TestBulkMemoryOperations_Interpreter(t *testing.T) {
	testBulkMemoryOperations(t, wazero.NewRuntimeConfigInterpreter)
	testTableCopy(t, wazero.NewRuntimeConfigInterpreter)
	testTableInit(t, wazero.NewRuntimeConfigInterpreter)
	testElemDrop(t, wazero.NewRuntimeConfigInterpreter)
}

var (
	// bulkMemoryOperationsWasm was compiled from testdata/bulk_memory_operations.wat
	//go:embed testdata/bulk_memory_operations.wasm
	bulkMemoryOperationsWasm []byte
	// tableCopyWasm was compiled from testdata/table_copy.wat
	//go:embed testdata/table_copy.wasm
	tableCopyWasm []byte
	// tableInitWasm was compiled from testdata/table_init.wat
	//go:embed testdata/table_init.wasm
	tableInitWasm []byte
	// elemDropWasm was compiled from testdata/elem_drop.wat
	//go:embed testdata/elem_drop.wasm
	elemDropWasm []byte
)

func requireErrorOnBulkMemoryFeatureDisabled(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig, bin []byte) {
	t.Run("disabled", func(t *testing.T) {
		// bulk-memory-operations is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(testCtx, bin)
		require.Error(t, err)
	})
}

func testTableCopy(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.copy", func(t *testing.T) {

		requireErrorOnBulkMemoryFeatureDisabled(t, newRuntimeConfig, tableCopyWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureBulkMemoryOperations(true))
		mod, err := r.InstantiateModuleFromCode(testCtx, tableCopyWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		// Non-overlapping copy.
		_, err = mod.ExportedFunction("copy").Call(testCtx, 3, 0, 3)
		require.NoError(t, err)
		res, err := mod.ExportedFunction("call").Call(testCtx, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(0), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 4)
		require.NoError(t, err)
		require.Equal(t, uint64(1), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 5)
		require.NoError(t, err)
		require.Equal(t, uint64(2), res[0])

		// src > dest with overlap
		_, err = mod.ExportedFunction("copy").Call(testCtx, 0, 1, 3)
		require.NoError(t, err)
		res, err = mod.ExportedFunction("call").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(2), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(0), res[0])

		// src < dest with overlap
		_, err = mod.ExportedFunction("copy").Call(testCtx, 2, 0, 3)
		require.NoError(t, err)
		res, err = mod.ExportedFunction("call").Call(testCtx, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(1), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(2), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 4)
		require.NoError(t, err)
		require.Equal(t, uint64(0), res[0])

		// Copying end at limit should be fine.
		_, err = mod.ExportedFunction("copy").Call(testCtx, 6, 8, 2)
		require.NoError(t, err)
		_, err = mod.ExportedFunction("copy").Call(testCtx, 8, 6, 2)
		require.NoError(t, err)

		// Copying zero size at the end of region is valid.
		_, err = mod.ExportedFunction("copy").Call(testCtx, 10, 0, 0)
		require.NoError(t, err)
		_, err = mod.ExportedFunction("copy").Call(testCtx, 0, 10, 0)
		require.NoError(t, err)

		// Out of bounds with size zero on outside of table.
		_, err = mod.ExportedFunction("copy").Call(testCtx, 11, 0, 0)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
		_, err = mod.ExportedFunction("copy").Call(testCtx, 0, 11, 0)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
	})
}

func testTableInit(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.init", func(t *testing.T) {
		requireErrorOnBulkMemoryFeatureDisabled(t, newRuntimeConfig, tableInitWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureBulkMemoryOperations(true))
		mod, err := r.InstantiateModuleFromCode(testCtx, tableInitWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		// Out of bounds access should raise the runtime error.
		_, err = mod.ExportedFunction("init").Call(testCtx, 2, 0, 2)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
		// And the table still not initialized.
		_, err = mod.ExportedFunction("call").Call(testCtx, 2)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)

		_, err = mod.ExportedFunction("init").Call(testCtx, 0, 1, 2)
		require.NoError(t, err)
		res, err := mod.ExportedFunction("call").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), res[0])
		res, err = mod.ExportedFunction("call").Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(0), res[0])

		// Initialization ending at the limit should be fine.
		_, err = mod.ExportedFunction("init").Call(testCtx, 1, 2, 2)
		require.NoError(t, err)
		// Also, zero length at the end also fine.
		_, err = mod.ExportedFunction("init").Call(testCtx, 3, 0, 0)
		require.NoError(t, err)
		_, err = mod.ExportedFunction("init").Call(testCtx, 0, 4, 0)
		require.NoError(t, err)

		// Initialization out side of table with size zero should be trap.
		_, err = mod.ExportedFunction("init").Call(testCtx, 4, 0, 0)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
		// Same goes for element.
		_, err = mod.ExportedFunction("init").Call(testCtx, 0, 5, 0)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
	})
}

func testElemDrop(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("elem.drop", func(t *testing.T) {
		requireErrorOnBulkMemoryFeatureDisabled(t, newRuntimeConfig, elemDropWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureBulkMemoryOperations(true))
		mod, err := r.InstantiateModuleFromCode(testCtx, elemDropWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		// Copying the passive element $a into index zero at the table.
		_, err = mod.ExportedFunction("init_passive").Call(testCtx, 1)
		require.NoError(t, err)
		// Droppig same elements should be fine.
		_, err = mod.ExportedFunction("drop_passive").Call(testCtx)
		require.NoError(t, err)
		_, err = mod.ExportedFunction("drop_passive").Call(testCtx)
		require.NoError(t, err)

		// Size zero init access to the size zero (dropped) elements should be ok.
		_, err = mod.ExportedFunction("init_passive").Call(testCtx, 0)
		require.NoError(t, err)

		// Buf size must be zero for such dropped elements.
		_, err = mod.ExportedFunction("init_passive").Call(testCtx, 1)
		require.ErrorIs(t, err, wasmruntime.ErrRuntimeInvalidTableAccess)
	})
}

func testBulkMemoryOperations(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	requireErrorOnBulkMemoryFeatureDisabled(t, newRuntimeConfig, bulkMemoryOperationsWasm)

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
