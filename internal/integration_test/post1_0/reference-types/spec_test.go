package referencetypes

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestReferenceTypes_JIT(t *testing.T) {
	testCallIndirectMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	tableCopyMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	tableInitMultiTables(t, wazero.NewRuntimeConfigInterpreter)
}

func TestReferenceTypes_Interpreter(t *testing.T) {
	testCallIndirectMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	tableCopyMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	tableInitMultiTables(t, wazero.NewRuntimeConfigInterpreter)
}

var (
	// callIndirectWasm was compiled from testdata/call_indirect_multi.wat
	//go:embed testdata/call_indirect_multi.wasm
	callIndirectMultiWasm []byte
	// tableCopyMultiWasm was compiled from testdata/table_copy_multi.wat
	//go:embed testdata/table_copy_multi.wasm
	tableCopyMultiWasm []byte
	// tableInitMultiWasm was compiled from testdata/table_init_multi.wat
	//go:embed testdata/table_init_multi.wasm
	tableInitMultiWasm []byte
)

func requireErrorDisabled(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig, bin []byte) {
	t.Run("disabled", func(t *testing.T) {
		// reference-types is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(testCtx, bin)
		require.Error(t, err)
	})
}

func tableCopyMultiTables(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.copy multi tables", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableCopyMultiWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureBulkMemoryOperations(true).
			WithFeatureReferenceTypes(true))

		a, err := r.NewModuleBuilder("a").
			ExportFunctions(map[string]interface{}{
				"ef0": func() uint32 { return 0 },
				"ef1": func() uint32 { return 1 },
				"ef2": func() uint32 { return 2 },
				"ef3": func() uint32 { return 3 },
				"ef4": func() uint32 { return 4 },
			}).Instantiate(testCtx)
		require.NoError(t, err)
		defer a.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableCopyMultiWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		_, err = mod.ExportedFunction("test").Call(testCtx)
		require.NoError(t, err)

		requireError := func(funcName string, in uint32) {
			_, err = mod.ExportedFunction(funcName).Call(testCtx, uint64(in))
			require.Error(t, err)
		}
		requireReturn := func(funcName string, in, exp uint32) {
			actual, err := mod.ExportedFunction(funcName).Call(testCtx, uint64(in))
			require.NoError(t, err)
			require.Equal(t, exp, uint32(actual[0]))
		}

		// t0
		for i := uint32(0); i <= 1; i++ {
			requireError("check_t0", i)
		}
		for _, tc := range [][2]uint32{{2, 3}, {3, 1}, {4, 4}, {5, 1}} {
			requireReturn("check_t0", tc[0], tc[1])
		}
		for i := uint32(6); i <= 11; i++ {
			requireError("check_t0", i)
		}
		for _, tc := range [][2]uint32{{12, 7}, {13, 5}, {14, 2}, {15, 3}, {16, 6}} {
			requireReturn("check_t0", tc[0], tc[1])
		}
		for i := uint32(17); i <= 29; i++ {
			requireError("check_t0", i)
		}

		// t1
		for i := uint32(0); i <= 2; i++ {
			requireError("check_t1", i)
		}
		for _, tc := range [][2]uint32{{3, 1}, {4, 3}, {5, 1}, {6, 4}} {
			requireReturn("check_t1", tc[0], tc[1])
		}
		for i := uint32(7); i <= 10; i++ {
			requireError("check_t1", i)
		}
		for _, tc := range [][2]uint32{{11, 6}, {12, 3}, {13, 2}, {14, 5}, {15, 7}} {
			requireReturn("check_t1", tc[0], tc[1])
		}
		for i := uint32(16); i <= 29; i++ {
			requireError("check_t1", i)
		}
	})
}

func tableInitMultiTables(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.init multi tables", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableInitMultiWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureBulkMemoryOperations(true).
			WithFeatureReferenceTypes(true))

		a, err := r.NewModuleBuilder("a").
			ExportFunctions(map[string]interface{}{
				"ef0": func() uint32 { return 0 },
				"ef1": func() uint32 { return 1 },
				"ef2": func() uint32 { return 2 },
				"ef3": func() uint32 { return 3 },
				"ef4": func() uint32 { return 4 },
			}).Instantiate(testCtx)
		require.NoError(t, err)
		defer a.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableInitMultiWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		_, err = mod.ExportedFunction("test").Call(testCtx)
		require.NoError(t, err)

		checkFn := mod.ExportedFunction("check")
		require.NotNil(t, checkFn)

		requireError := func(in uint32) {
			_, err = checkFn.Call(testCtx, uint64(in))
			require.Error(t, err)
		}
		requireReturn := func(in, exp uint32) {
			actual, err := checkFn.Call(testCtx, uint64(in))
			require.NoError(t, err)
			require.Equal(t, exp, uint32(actual[0]))
		}

		for i := uint32(0); i <= 1; i++ {
			requireError(i)
		}
		for _, tc := range [][2]uint32{{2, 3}, {3, 1}, {4, 4}, {5, 1}} {
			requireReturn(tc[0], tc[1])
		}
		requireError(6)
		for _, tc := range [][2]uint32{{7, 2}, {8, 7}, {9, 1}, {10, 8}} {
			requireReturn(tc[0], tc[1])
		}
		requireError(11)
		for _, tc := range [][2]uint32{{12, 7}, {13, 5}, {14, 2}, {15, 3}, {16, 6}} {
			requireReturn(tc[0], tc[1])
		}
		for i := uint32(17); i <= 29; i++ {
			requireError(i)
		}
	})
}

func testCallIndirectMultiTables(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("call_indirect multi tables", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, callIndirectMultiWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureBulkMemoryOperations(true).
			WithFeatureReferenceTypes(true))
		mod, err := r.InstantiateModuleFromCode(testCtx, callIndirectMultiWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		actual, err := mod.ExportedFunction("call-1").Call(testCtx, 2, 3, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(5), actual[0])

		actual, err = mod.ExportedFunction("call-1").Call(testCtx, 2, 3, 1)
		require.NoError(t, err)
		require.Equal(t, int32(-1), int32(actual[0]))

		_, err = mod.ExportedFunction("call-1").Call(testCtx, 2, 3, 2)
		require.Error(t, err)

		actual, err = mod.ExportedFunction("call-2").Call(testCtx, 2, 3, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(6), actual[0])

		actual, err = mod.ExportedFunction("call-2").Call(testCtx, 2, 3, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		actual, err = mod.ExportedFunction("call-2").Call(testCtx, 2, 3, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(2), actual[0])

		_, err = mod.ExportedFunction("call-2").Call(testCtx, 2, 3, 3)
		require.Error(t, err)

		actual, err = mod.ExportedFunction("call-3").Call(testCtx, 2, 3, 0)
		require.NoError(t, err)
		require.Equal(t, int32(-1), int32(actual[0]))

		actual, err = mod.ExportedFunction("call-3").Call(testCtx, 2, 3, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(6), actual[0])

		_, err = mod.ExportedFunction("call-3").Call(testCtx, 2, 3, 2)
		require.Error(t, err)

		_, err = mod.ExportedFunction("call-3").Call(testCtx, 2, 3, 3)
		require.Error(t, err)

		_, err = mod.ExportedFunction("call-3").Call(testCtx, 2, 3, 4)
		require.Error(t, err)
	})
}
