package multi_value

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

func TestMultiValue_JIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	testMultiValue(t, wazero.NewRuntimeConfigJIT)
}

func TestMultiValue_Interpreter(t *testing.T) {
	testMultiValue(t, wazero.NewRuntimeConfigInterpreter)
}

// multiValueWasm was compiled from testdata/multi_value.wat
//go:embed testdata/multi_value.wasm
var multiValueWasm []byte

func testMultiValue(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	t.Run("disabled", func(t *testing.T) {
		// multi-value is disabled by default.
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig())
		_, err := r.InstantiateModuleFromCode(multiValueWasm)
		require.Error(t, err)
	})
	t.Run("enabled", func(t *testing.T) {
		r := wazero.NewRuntimeWithConfig(newRuntimeConfig().WithFeatureMultiValue(true))
		module, err := r.InstantiateModuleFromCode(multiValueWasm)
		require.NoError(t, err)
		defer module.Close()

		swap := module.ExportedFunction("swap")
		results, err := swap.Call(nil, 100, 200)
		require.NoError(t, err)
		require.Equal(t, []uint64{200, 100}, results)

		add64UWithCarry := module.ExportedFunction("add64_u_with_carry")
		results, err = add64UWithCarry.Call(nil, 0x8000000000000000, 0x8000000000000000, 0)
		require.NoError(t, err)
		require.Equal(t, []uint64{0, 1}, results)

		add64USaturated := module.ExportedFunction("add64_u_saturated")
		results, err = add64USaturated.Call(nil, 1230, 23)
		require.NoError(t, err)
		require.Equal(t, []uint64{1253}, results)

		fac := module.ExportedFunction("fac")
		results, err = fac.Call(nil, 25)
		require.NoError(t, err)
		require.Equal(t, []uint64{7034535277573963776}, results)

		t.Run("br.wast", func(t *testing.T) {
			testBr(t, r)
		})
		t.Run("call.wast", func(t *testing.T) {
			testCall(t, r)
		})
		t.Run("call_indirect.wast", func(t *testing.T) {
			testCallIndirect(t, r)
		})
		t.Run("fac.wast", func(t *testing.T) {
			testFac(t, r)
		})
		t.Run("func.wast", func(t *testing.T) {
			testFunc(t, r)
		})
		t.Run("if.wast", func(t *testing.T) {
			testIf(t, r)
		})
		t.Run("loop.wast", func(t *testing.T) {
			testLoop(t, r)
		})
	})
}

// brWasm was compiled from testdata/br.wat
//go:embed testdata/br.wasm
var brWasm []byte

func testBr(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(brWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "type-i32-i32"}, {name: "type-i64-i64"}, {name: "type-f32-f32"}, {name: "type-f64-f64"},
		{name: "type-f64-f64-value", expected: []uint64{api.EncodeF64(4), api.EncodeF64(5)}},
		{name: "as-return-values", expected: []uint64{2, 7}},
		{name: "as-select-all", expected: []uint64{8}},
		{name: "as-call-all", expected: []uint64{15}},
		{name: "as-call_indirect-all", expected: []uint64{24}},
		{name: "as-store-both", expected: []uint64{32}},
		{name: "as-storeN-both", expected: []uint64{34}},
		{name: "as-binary-both", expected: []uint64{46}},
		{name: "as-compare-both", expected: []uint64{44}},
	})
}

// callWasm was compiled from testdata/call.wat
//go:embed testdata/call.wasm
var callWasm []byte

func testCall(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(callWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "type-i32-i64", expected: []uint64{0x132, 0x164}},
		{name: "type-all-i32-f64", expected: []uint64{32, api.EncodeF64(1.64)}},
		{name: "type-all-i32-i32", expected: []uint64{2, 1}},
		{name: "type-all-f32-f64", expected: []uint64{api.EncodeF64(2), api.EncodeF32(1)}},
		{name: "type-all-f64-i32", expected: []uint64{2, api.EncodeF64(1)}},
		{name: "as-binary-all-operands", expected: []uint64{7}},
		{name: "as-mixed-operands", expected: []uint64{32}},
		{name: "as-call-all-operands", expected: []uint64{3, 4}},
	})
}

// callIndirectWasm was compiled from testdata/call_indirect.wat
//go:embed testdata/call_indirect.wasm
var callIndirectWasm []byte

func testCallIndirect(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(callIndirectWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "type-f64-i32", expected: []uint64{api.EncodeF64(0xf64), 32}},
		{name: "type-all-f64-i32", expected: []uint64{api.EncodeF64(0xf64), 32}},
		{name: "type-all-i32-f64", expected: []uint64{1, api.EncodeF64(2)}},
		{name: "type-all-i32-i64", expected: []uint64{2, 1}},
	})

	_, err = module.ExportedFunction("dispatch").Call(nil, 32, 2)
	require.EqualError(t, err, `wasm error: invalid table access
wasm stack trace:
	call_indirect.wast.[16](i32,i64) i64`)
}

// facWasm was compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

func testFac(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(facWasm)
	require.NoError(t, err)
	defer module.Close()

	fac := module.ExportedFunction("fac-ssa")
	results, err := fac.Call(nil, 25)
	require.NoError(t, err)
	require.Equal(t, []uint64{7034535277573963776}, results)
}

// funcWasm was compiled from testdata/func.wat
//go:embed testdata/func.wasm
var funcWasm []byte

func testFunc(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(funcWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "value-i32-f64", expected: []uint64{77, api.EncodeF64(7)}},
		{name: "value-i32-i32-i32", expected: []uint64{1, 2, 3}},
		{name: "value-block-i32-i64", expected: []uint64{1, 2}},

		{name: "return-i32-f64", expected: []uint64{78, api.EncodeF64(78.78)}},
		{name: "return-i32-i32-i32", expected: []uint64{1, 2, 3}},
		{name: "return-block-i32-i64", expected: []uint64{1, 2}},
		{name: "break-i32-f64", expected: []uint64{79, api.EncodeF64(79.79)}},
		{name: "break-i32-i32-i32", expected: []uint64{1, 2, 3}},
		{name: "break-block-i32-i64", expected: []uint64{1, 2}},

		{name: "break-br_if-num-num", params: []uint64{0}, expected: []uint64{51, 52}},
		{name: "break-br_if-num-num", params: []uint64{1}, expected: []uint64{50, 51}},
		{name: "break-br_table-num-num", params: []uint64{0}, expected: []uint64{50, 51}},
		{name: "break-br_table-num-num", params: []uint64{1}, expected: []uint64{50, 51}},
		{name: "break-br_table-num-num", params: []uint64{10}, expected: []uint64{50, 51}},
		{name: "break-br_table-num-num", params: []uint64{api.EncodeI32(-100)}, expected: []uint64{50, 51}},
		{name: "break-br_table-nested-num-num", params: []uint64{0}, expected: []uint64{101, 52}},
		{name: "break-br_table-nested-num-num", params: []uint64{1}, expected: []uint64{50, 51}},
		{name: "break-br_table-nested-num-num", params: []uint64{2}, expected: []uint64{101, 52}},
		{name: "break-br_table-nested-num-num", params: []uint64{api.EncodeI32(-3)}, expected: []uint64{101, 52}},
	})

	fac := module.ExportedFunction("large-sig")
	results, err := fac.Call(nil,
		0, 1, api.EncodeF32(2), api.EncodeF32(3),
		4, api.EncodeF64(5), api.EncodeF32(6), 7,
		8, 9, api.EncodeF32(10), api.EncodeF64(11),
		api.EncodeF64(12), api.EncodeF64(13), 14, 15,
		api.EncodeF32(16))
	require.NoError(t, err)
	require.Equal(t, []uint64{api.EncodeF64(5), api.EncodeF32(2), 0, 8,
		7, 1, api.EncodeF32(3), 9,
		4, api.EncodeF32(6), api.EncodeF64(13), api.EncodeF64(11),
		15, api.EncodeF32(16), 14, api.EncodeF64(12),
	}, results)
}

// ifWasm was compiled from testdata/if.wat
//go:embed testdata/if.wasm
var ifWasm []byte

func testIf(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(ifWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "multi", params: []uint64{0}, expected: []uint64{9, api.EncodeI32(-1)}},
		{name: "multi", params: []uint64{1}, expected: []uint64{8, 1}},
		{name: "multi", params: []uint64{13}, expected: []uint64{8, 1}},
		{name: "multi", params: []uint64{api.EncodeI32(-5)}, expected: []uint64{8, 1}},
		{name: "as-binary-operands", params: []uint64{0}, expected: []uint64{api.EncodeI32(-12)}},
		{name: "as-binary-operands", params: []uint64{1}, expected: []uint64{api.EncodeI32(12)}},
		{name: "as-compare-operands", params: []uint64{0}, expected: []uint64{1}},
		{name: "as-compare-operands", params: []uint64{1}, expected: []uint64{0}},
		{name: "as-mixed-operands", params: []uint64{0}, expected: []uint64{api.EncodeI32(-3)}},
		{name: "as-mixed-operands", params: []uint64{1}, expected: []uint64{27}},
		{name: "break-multi-value", params: []uint64{0}, expected: []uint64{api.EncodeI32(-18), 18, api.EncodeI64(-18)}},
		{name: "break-multi-value", params: []uint64{1}, expected: []uint64{18, api.EncodeI32(-18), 18}},
		{name: "param", params: []uint64{0}, expected: []uint64{api.EncodeI32(-1)}},
		{name: "param", params: []uint64{1}, expected: []uint64{3}},
		{name: "params", params: []uint64{0}, expected: []uint64{api.EncodeI32(-1)}},
		{name: "params", params: []uint64{1}, expected: []uint64{3}},
		{name: "params-id", params: []uint64{0}, expected: []uint64{3}},
		{name: "params-id", params: []uint64{1}, expected: []uint64{3}},
		{name: "param-break", params: []uint64{0}, expected: []uint64{api.EncodeI32(-1)}},
		{name: "param-break", params: []uint64{1}, expected: []uint64{3}},
		{name: "params-break", params: []uint64{0}, expected: []uint64{api.EncodeI32(-1)}},
		{name: "params-break", params: []uint64{1}, expected: []uint64{3}},
		{name: "params-id-break", params: []uint64{0}, expected: []uint64{3}},
		{name: "params-id-break", params: []uint64{1}, expected: []uint64{3}},
		{name: "add64_u_with_carry", params: []uint64{0, 0, 0}, expected: []uint64{0, 0}},
		{name: "add64_u_with_carry", params: []uint64{100, 124, 0}, expected: []uint64{224, 0}},
		{name: "add64_u_with_carry", params: []uint64{api.EncodeI64(-1), 0, 0}, expected: []uint64{api.EncodeI64(-1), 0}},
		{name: "add64_u_with_carry", params: []uint64{api.EncodeI64(-1), 1, 0}, expected: []uint64{0, 1}},
		{name: "add64_u_with_carry", params: []uint64{api.EncodeI64(-1), api.EncodeI64(-1), 0}, expected: []uint64{api.EncodeI64(-2), 1}},
		{name: "add64_u_with_carry", params: []uint64{api.EncodeI64(-1), 0, 1}, expected: []uint64{0, 1}},
		{name: "add64_u_with_carry", params: []uint64{api.EncodeI64(-1), 1, 1}, expected: []uint64{1, 1}},
		{name: "add64_u_with_carry", params: []uint64{0x8000000000000000, 0x8000000000000000, 0}, expected: []uint64{0, 1}},
		{name: "add64_u_saturated", params: []uint64{0, 0}, expected: []uint64{0}},
		{name: "add64_u_saturated", params: []uint64{1230, 23}, expected: []uint64{1253}},
		{name: "add64_u_saturated", params: []uint64{api.EncodeI64(-1), 0}, expected: []uint64{api.EncodeI64(-1)}},
		{name: "add64_u_saturated", params: []uint64{api.EncodeI64(-1), 1}, expected: []uint64{api.EncodeI64(-1)}},
		{name: "add64_u_saturated", params: []uint64{api.EncodeI64(-1), api.EncodeI64(-1)}, expected: []uint64{api.EncodeI64(-1)}},
		{name: "add64_u_saturated", params: []uint64{0x8000000000000000, 0x8000000000000000}, expected: []uint64{api.EncodeI64(-1)}},
		{name: "type-use"},
	})
}

// loopWasm was compiled from testdata/loop.wat
//go:embed testdata/loop.wasm
var loopWasm []byte

func testLoop(t *testing.T, r wazero.Runtime) {
	module, err := r.InstantiateModuleFromCode(loopWasm)
	require.NoError(t, err)
	defer module.Close()

	testFunctions(t, module, []funcTest{
		{name: "as-binary-operands", expected: []uint64{12}},
		{name: "as-compare-operands", expected: []uint64{0}},
		{name: "as-mixed-operands", expected: []uint64{27}},
		{name: "break-multi-value", expected: []uint64{18, api.EncodeI32(-18), 18}},
		{name: "param", expected: []uint64{3}},
		{name: "params", expected: []uint64{3}},
		{name: "params-id", expected: []uint64{3}},
		{name: "param-break", expected: []uint64{13}},
		{name: "params-break", expected: []uint64{12}},
		{name: "params-id-break", expected: []uint64{3}},
		{name: "type-use"},
	})
}

type funcTest struct {
	name     string
	params   []uint64
	expected []uint64
}

func testFunctions(t *testing.T, module api.Module, tests []funcTest) {
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			results, err := module.ExportedFunction(tc.name).Call(nil, tc.params...)
			require.NoError(t, err)
			if tc.expected == nil {
				require.Empty(t, results)
			} else {
				require.Equal(t, tc.expected, results)
			}
		})
	}
}
