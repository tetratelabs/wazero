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
	if !wazero.JITSupported {
		t.Skip()
	}
	testCallIndirectMultiTables(t, wazero.NewRuntimeConfigJIT)
	testTableCopyMultiTables(t, wazero.NewRuntimeConfigJIT)
	testTableInitMultiTables(t, wazero.NewRuntimeConfigJIT)
	testRefNull(t, wazero.NewRuntimeConfigJIT)
	testRefIsNull(t, wazero.NewRuntimeConfigJIT)
	testRefFunc(t, wazero.NewRuntimeConfigJIT)
	testTableSet(t, wazero.NewRuntimeConfigJIT)
	testTableGet(t, wazero.NewRuntimeConfigJIT)
	testTableGrow(t, wazero.NewRuntimeConfigJIT)
	testTableFill(t, wazero.NewRuntimeConfigJIT)
}

func TestReferenceTypes_Interpreter(t *testing.T) {
	testCallIndirectMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	testTableCopyMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	testTableInitMultiTables(t, wazero.NewRuntimeConfigInterpreter)
	testRefNull(t, wazero.NewRuntimeConfigInterpreter)
	testRefIsNull(t, wazero.NewRuntimeConfigInterpreter)
	testRefFunc(t, wazero.NewRuntimeConfigInterpreter)
	testTableSet(t, wazero.NewRuntimeConfigInterpreter)
	testTableGet(t, wazero.NewRuntimeConfigInterpreter)
	testTableGrow(t, wazero.NewRuntimeConfigInterpreter)
	testTableFill(t, wazero.NewRuntimeConfigInterpreter)
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
	// refNullWasm was compiled from testdata/ref_null.wat
	//go:embed testdata/ref_null.wasm
	refNullWasm []byte
	// refIsNullWasm was compiled from testdata/ref_is_null.wat
	//go:embed testdata/ref_is_null.wasm
	refIsNullWasm []byte
	// refFunclWasm was compiled from testdata/ref_func.wat
	//go:embed testdata/ref_func.wasm
	refFunclWasm []byte
	// refFunclWasm was compiled from testdata/ref_func_loadable.wat
	//go:embed testdata/ref_func_loadable.wasm
	refFuncLoadableWasm []byte
	// refFuncInvalideWasm was compiled from testdata/ref_func_invalid.wat
	//go:embed testdata/ref_func_invalid.wasm
	refFuncInvalideWasm []byte
	// tableSetWasm was compiled from testdata/table_set.wat
	//go:embed testdata/table_set.wasm
	tableSetWasm []byte
	// tableSetInvalidWasm was compiled from testdata/table_set_invalid.wat
	//go:embed testdata/table_set_invalid.wasm
	tableSetInvalidWasm []byte
	// tableGetWasm was compiled from testdata/table_get.wat
	//go:embed testdata/table_get.wasm
	tableGetWasm []byte
	// tableGrowWasm was compiled from testdata/table_grow.wat
	//go:embed testdata/table_grow.wasm
	tableGrowWasm []byte
	// tableGrowCheckNullWasm was compiled from testdata/table_grow_check_null.wat
	//go:embed testdata/table_grow_check_null.wasm
	tableGrowCheckNullWasm []byte
	// tableFillWasm was compiled from testdata/table_fill.wat
	//go:embed testdata/table_fill.wasm
	tableFillWasm []byte
)

func requireErrorDisabled(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig, bin []byte) {
	t.Run("disabled", func(t *testing.T) {
		// reference-types is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(testCtx, bin)
		require.Error(t, err)
	})
}

func testTableCopyMultiTables(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.copy multi tables", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableCopyMultiWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureBulkMemoryOperations(true).
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		_, err := r.NewModuleBuilder("a").
			ExportFunctions(map[string]interface{}{
				"ef0": func() uint32 { return 0 },
				"ef1": func() uint32 { return 1 },
				"ef2": func() uint32 { return 2 },
				"ef3": func() uint32 { return 3 },
				"ef4": func() uint32 { return 4 },
			}).Instantiate(testCtx)
		require.NoError(t, err)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableCopyMultiWasm)
		require.NoError(t, err)

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

func testTableFill(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.fill", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableFillWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableFillWasm)
		require.NoError(t, err)

		get, fill := mod.ExportedFunction("get"), mod.ExportedFunction("fill")
		require.NotNil(t, get)
		require.NotNil(t, fill)

		reqireGet := func(in, exp uint64) {
			actual, err := get.Call(testCtx, in)
			require.NoError(t, err)
			require.Equal(t, exp, actual[0])
		}

		requireFill := func(params ...uint64) {
			_, err := fill.Call(testCtx, params...)
			require.NoError(t, err)
		}

		nullRef := uint64(0)
		reqireGet(1, nullRef)
		reqireGet(2, nullRef)
		reqireGet(3, nullRef)
		reqireGet(4, nullRef)
		reqireGet(5, nullRef)

		nonNullRefX := uint64(1)
		requireFill(2, nonNullRefX, 3)
		reqireGet(1, nullRef)
		reqireGet(2, nonNullRefX)
		reqireGet(3, nonNullRefX)
		reqireGet(4, nonNullRefX)
		reqireGet(5, nullRef)

		nonNullRefY := uint64(2)
		requireFill(4, nonNullRefY, 2)
		reqireGet(3, nonNullRefX)
		reqireGet(4, nonNullRefY)
		reqireGet(5, nonNullRefY)
		reqireGet(6, nullRef)

		nonNullRefZ := uint64(3)
		requireFill(4, nonNullRefZ, 0)
		reqireGet(3, nonNullRefX)
		reqireGet(4, nonNullRefY)
		reqireGet(5, nonNullRefY)

		nonNullRefA := uint64(4)
		requireFill(8, nonNullRefA, 2)
		reqireGet(7, nullRef)
		reqireGet(8, nonNullRefA)
		reqireGet(9, nonNullRefA)

		requireFill(9, nullRef, 1)
		reqireGet(8, nonNullRefA)
		reqireGet(9, nullRef)

		nonNullRefB := uint64(5)
		requireFill(10, nonNullRefB, 0)
		reqireGet(9, nullRef)

		nonNullRefC := uint64(6)
		_, err = fill.Call(testCtx, 8, nonNullRefC, 3)
		require.Error(t, err)

		reqireGet(7, nullRef)
		reqireGet(8, nonNullRefA)
		reqireGet(9, nullRef)

		_, err = fill.Call(testCtx, 11, nullRef, 0)
		require.Error(t, err)
		_, err = fill.Call(testCtx, 11, nullRef, 10)
		require.Error(t, err)
	})
}

func testTableGrow(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.grow check null", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableGrowCheckNullWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableGrowCheckNullWasm)
		require.NoError(t, err)

		nullRef := uint64(0)
		actual, err := mod.ExportedFunction("check-table-null").Call(testCtx, 0, 9)
		require.NoError(t, err)
		require.Equal(t, nullRef, actual[0])

		actual, err = mod.ExportedFunction("grow").Call(testCtx, 10)
		require.NoError(t, err)
		require.Equal(t, uint64(10), actual[0])

		actual, err = mod.ExportedFunction("check-table-null").Call(testCtx, 0, 19)
		require.NoError(t, err)
		require.Equal(t, nullRef, actual[0])
	})

	t.Run("table.grow", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableGrowWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableGrowWasm)
		require.NoError(t, err)

		size, get, set, grow := mod.ExportedFunction("size"), mod.ExportedFunction("get"),
			mod.ExportedFunction("set"), mod.ExportedFunction("grow")
		require.NotNil(t, size)
		require.NotNil(t, get)
		require.NotNil(t, set)
		require.NotNil(t, grow)

		actual, err := size.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		_, err = set.Call(testCtx, 0, 2)
		require.Error(t, err)
		_, err = get.Call(testCtx, 0)
		require.Error(t, err)

		nullRef := uint64(0)
		actual, err = grow.Call(testCtx, 1, nullRef)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		actual, err = size.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = get.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRef, actual[0])

		nonNullRefX := uint64(2)
		_, err = set.Call(testCtx, 0, nonNullRefX)
		require.NoError(t, err)

		actual, err = get.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nonNullRefX, actual[0])

		_, err = set.Call(testCtx, 1, nonNullRefX)
		require.Error(t, err)
		_, err = get.Call(testCtx, 1)
		require.Error(t, err)

		nonNullRefY := uint64(3)
		actual, err = grow.Call(testCtx, 4, nonNullRefY)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = size.Call(testCtx)
		require.NoError(t, err)
		require.Equal(t, uint64(5), actual[0])

		actual, err = get.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nonNullRefX, actual[0])

		_, err = set.Call(testCtx, 0, nonNullRefX)
		require.NoError(t, err)

		actual, err = get.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nonNullRefX, actual[0])

		actual, err = get.Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, nonNullRefY, actual[0])

		actual, err = get.Call(testCtx, 4)
		require.NoError(t, err)
		require.Equal(t, nonNullRefY, actual[0])

		nonNullRefZ := uint64(4)
		_, err = set.Call(testCtx, 4, nonNullRefZ)
		require.NoError(t, err)

		actual, err = get.Call(testCtx, 4)
		require.NoError(t, err)
		require.Equal(t, nonNullRefZ, actual[0])

		_, err = set.Call(testCtx, 5, nonNullRefX)
		require.Error(t, err)
		_, err = get.Call(testCtx, 5)
		require.Error(t, err)
	})
}

func testTableGet(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.get", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableGetWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableGetWasm)
		require.NoError(t, err)

		externRef := uint64(1)
		_, err = mod.ExportedFunction("init").Call(testCtx, externRef)
		require.NoError(t, err)

		nullRefAsUint64 := uint64(0)

		actual, err := mod.ExportedFunction("get-externref").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		actual, err = mod.ExportedFunction("get-externref").Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, externRef, actual[0])

		actual, err = mod.ExportedFunction("get-funcref").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		actual, err = mod.ExportedFunction("is_null-funcref").Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		actual, err = mod.ExportedFunction("is_null-funcref").Call(testCtx, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		requireTrap := func(fn string, params ...uint64) {
			actual, err = mod.ExportedFunction(fn).Call(testCtx, params...)
			require.Error(t, err)
		}

		requireTrap("get-externref", 2)
		requireTrap("get-funcref", 3)
		requireTrap("get-externref", uint64(0xffffffff))
		requireTrap("get-funcref", uint64(0xffffffff))
	})
}

func testTableSet(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.set - invalid", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		_, err := r.InstantiateModuleFromCode(testCtx, tableSetInvalidWasm)
		require.EqualError(t, err, "invalid function[0]: cannot pop the operand for table.set: type mismatch: expected funcref, but was externref")
	})
	t.Run("table.set", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableSetWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableSetWasm)
		require.NoError(t, err)

		getExternref, setExternref := mod.ExportedFunction("get-externref"), mod.ExportedFunction("set-externref")
		require.NotNil(t, getExternref)
		require.NotNil(t, setExternref)

		nullRefAsUint64 := uint64(0)
		actual, err := getExternref.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		nonNullExternRefAsUint64 := uint64(1)
		_, err = setExternref.Call(testCtx, 0, nonNullExternRefAsUint64)
		require.NoError(t, err)

		actual, err = getExternref.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nonNullExternRefAsUint64, actual[0])

		_, err = setExternref.Call(testCtx, 0, nullRefAsUint64)
		require.NoError(t, err)

		actual, err = getExternref.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		actual, err = mod.ExportedFunction("get-funcref").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		_, err = mod.ExportedFunction("set-funcref-from").Call(testCtx, 0, 1)
		require.NoError(t, err)

		actual, err = mod.ExportedFunction("is_null-funcref").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		_, err = mod.ExportedFunction("set-funcref").Call(testCtx, 0, nullRefAsUint64)
		require.NoError(t, err)

		actual, err = mod.ExportedFunction("get-funcref").Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, nullRefAsUint64, actual[0])

		requireTrap := func(fn string, params ...uint64) {
			actual, err = mod.ExportedFunction(fn).Call(testCtx, params...)
			require.Error(t, err)
		}

		requireTrap("set-externref", 2, nullRefAsUint64)
		requireTrap("set-funcref", 3, nullRefAsUint64)
		requireTrap("set-externref", uint64(0xffffffff), nullRefAsUint64)
		requireTrap("set-funcref", uint64(0xffffffff), nullRefAsUint64)

		requireTrap("set-externref", 2, 0)
		requireTrap("set-funcref-from", 3, 1)
		requireTrap("set-externref", uint64(0xffffffff), 0)
		requireTrap("set-funcref-from", uint64(0xffffffff), 1)
	})
}

func testRefFunc(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("ref.func", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, refFunclWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		_, err := r.NewModuleBuilder("M").
			ExportFunctions(map[string]interface{}{
				"f": func(v uint32) uint32 { return v },
			}).Instantiate(testCtx)
		require.NoError(t, err)

		mod, err := r.InstantiateModuleFromCode(testCtx, refFunclWasm)
		require.NoError(t, err)

		requireResult := func(fn string, exp uint64, params ...uint64) {
			actual, err := mod.ExportedFunction(fn).Call(testCtx, params...)
			require.NoError(t, err)
			require.Equal(t, exp, actual[0])
		}

		requireResult("is_null-f", 0)
		requireResult("is_null-g", 0)
		requireResult("is_null-v", 0)

		requireResult("call-f", 4, 4)
		requireResult("call-g", 5, 4)
		requireResult("call-v", 4, 4)

		_, err = mod.ExportedFunction("set-g").Call(testCtx)
		require.NoError(t, err)

		requireResult("call-v", 5, 4)

		_, err = mod.ExportedFunction("set-f").Call(testCtx)
		require.NoError(t, err)

		requireResult("call-v", 4, 4)
	})
	t.Run("ref.func - loadable", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, refFuncLoadableWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		mod, err := r.InstantiateModuleFromCode(testCtx, refFuncLoadableWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)
	})
	t.Run("ref.func - invalid module", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		_, err := r.InstantiateModuleFromCode(testCtx, refFuncInvalideWasm)
		require.EqualError(t, err, "ref.func index out of range [7] with length 1")
	})
}

func testRefNull(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("ref.null", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, refNullWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))

		mod, err := r.InstantiateModuleFromCode(testCtx, refNullWasm)
		require.NoError(t, err)
		defer mod.Close(testCtx)

		actual, err := mod.ExportedFunction("externref").Call(testCtx)
		require.NoError(t, err)
		// Null reference value should be translated as uint64(0).
		require.Equal(t, uint64(0), actual[0])

		actual, err = mod.ExportedFunction("funcref").Call(testCtx)
		require.NoError(t, err)
		// Null reference value should be translated as uint64(0).
		require.Equal(t, uint64(0), actual[0])
	})
}

func testRefIsNull(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("ref.is_null", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, refIsNullWasm)
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		mod, err := r.InstantiateModuleFromCode(testCtx, refIsNullWasm)
		require.NoError(t, err)

		funcrefFn := mod.ExportedFunction("funcref")
		require.NotNil(t, funcrefFn)
		externrefFn := mod.ExportedFunction("externref")
		require.NotNil(t, externrefFn)

		nullRefAsUint64 := uint64(0)
		actual, err := funcrefFn.Call(testCtx, nullRefAsUint64)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = externrefFn.Call(testCtx, nullRefAsUint64)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		nonNullExternRefAsUint64 := uint64(1)
		actual, err = externrefFn.Call(testCtx, nonNullExternRefAsUint64)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		_, err = mod.ExportedFunction("init").Call(testCtx, nonNullExternRefAsUint64)
		require.NoError(t, err)

		funcrefElemFn := mod.ExportedFunction("funcref-elem")
		require.NotNil(t, funcrefElemFn)
		externrefElemFn := mod.ExportedFunction("externref-elem")
		require.NotNil(t, externrefElemFn)

		actual, err = funcrefElemFn.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = externrefElemFn.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = funcrefElemFn.Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		actual, err = externrefElemFn.Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(0), actual[0])

		_, err = mod.ExportedFunction("deinit").Call(testCtx)
		require.NoError(t, err)

		actual, err = funcrefElemFn.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = externrefElemFn.Call(testCtx, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = funcrefElemFn.Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])

		actual, err = externrefElemFn.Call(testCtx, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(1), actual[0])
	})
}

func testTableInitMultiTables(t *testing.T, newRuntimeConfig func() wazero.RuntimeConfig) {
	t.Run("table.init multi tables", func(t *testing.T) {
		requireErrorDisabled(t, newRuntimeConfig, tableInitMultiWasm)

		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().
			WithFeatureBulkMemoryOperations(true).
			WithFeatureReferenceTypes(true))
		defer r.Close(testCtx)

		_, err := r.NewModuleBuilder("a").
			ExportFunctions(map[string]interface{}{
				"ef0": func() uint32 { return 0 },
				"ef1": func() uint32 { return 1 },
				"ef2": func() uint32 { return 2 },
				"ef3": func() uint32 { return 3 },
				"ef4": func() uint32 { return 4 },
			}).Instantiate(testCtx)
		require.NoError(t, err)

		mod, err := r.InstantiateModuleFromCode(testCtx, tableInitMultiWasm)
		require.NoError(t, err)

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
