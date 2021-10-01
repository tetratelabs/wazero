package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_local_tee(t *testing.T) {
	vm := requireInitVM(t, "local_tee", nil)

	assertReturnWithoutReturn := func(name string, args ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, args...)
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}

	assertReturn := func(name string, exp uint64, expType wasm.ValueType) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, expType, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	assertReturn2 := func(name string, arg, exp uint64, expType wasm.ValueType) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, expType, types[0])
			if expType == wasm.ValueTypeI32 {
				require.Equal(t, uint32(exp), uint32(values[0]))
			} else if expType == wasm.ValueTypeF32 {
				bothNaN := math.IsNaN(float64(math.Float32frombits(uint32(values[0])))) && math.IsNaN(float64(math.Float32frombits(uint32(exp))))
				if !bothNaN {
					require.Equal(t, exp, values[0])
				}
			} else {
				require.Equal(t, exp, values[0])

			}
		})
	}

	assertReturn3 := func(name string, arg1, arg2, exp uint64, expType wasm.ValueType) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg1, arg2)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, expType, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	assertReturn4 := func(name string, arg1, arg2, arg3, arg4, arg5, exp uint64, expType wasm.ValueType) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg1, arg2, arg3, arg4, arg5)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, expType, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	assertReturn("type-local-i32", uint64(int32(0)), wasm.ValueTypeI32)
	assertReturn("type-local-i64", uint64(int64(0)), wasm.ValueTypeI64)
	assertReturn("type-local-f32", uint64(math.Float32bits(0)), wasm.ValueTypeF32)
	assertReturn("type-local-f64", math.Float64bits(0), wasm.ValueTypeF64)

	assertReturn2("type-param-i32", uint64(int32(2)), uint64(int32(10)), wasm.ValueTypeI32)
	assertReturn2("type-param-i64", uint64(int64(3)), uint64(int64(11)), wasm.ValueTypeI64)
	assertReturn2("type-param-f32", uint64(math.Float32bits(4.4)), uint64(math.Float32bits(11.1)), wasm.ValueTypeF32)
	assertReturn2("type-param-f64", math.Float64bits(5.5), math.Float64bits(12.2), wasm.ValueTypeF64)

	assertReturn2("as-block-first", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)
	assertReturn2("as-block-mid", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)
	assertReturn2("as-block-last", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)

	assertReturn2("as-loop-first", uint64(int32(0)), uint64(int32(3)), wasm.ValueTypeI32)
	assertReturn2("as-loop-mid", uint64(int32(0)), uint64(int32(4)), wasm.ValueTypeI32)
	assertReturn2("as-loop-last", uint64(int32(0)), uint64(int32(5)), wasm.ValueTypeI32)

	assertReturn2("as-br-value", uint64(int32(0)), uint64(int32(9)), wasm.ValueTypeI32)

	assertReturnWithoutReturn("as-br_if-cond", uint64(int32(0)))
	assertReturn2("as-br_if-value", uint64(int32(0)), uint64(int32(8)), wasm.ValueTypeI32)
	assertReturn2("as-br_if-value-cond", uint64(int32(0)), uint64(int32(6)), wasm.ValueTypeI32)

	assertReturnWithoutReturn("as-br_table-index", uint64(int32(0)))
	assertReturn2("as-br_table-value", uint64(int32(0)), uint64(int32(10)), wasm.ValueTypeI32)
	assertReturn2("as-br_table-value-index", uint64(int32(0)), uint64(int32(6)), wasm.ValueTypeI32)

	assertReturn2("as-return-value", uint64(int32(0)), uint64(int32(7)), wasm.ValueTypeI32)

	assertReturn2("as-if-cond", uint64(int32(0)), uint64(int32(0)), wasm.ValueTypeI32)
	assertReturn2("as-if-then", uint64(int32(1)), uint64(int32(3)), wasm.ValueTypeI32)
	assertReturn2("as-if-else", uint64(int32(0)), uint64(int32(4)), wasm.ValueTypeI32)

	assertReturn3("as-select-first", uint64(int32(0)), uint64(int32(1)), uint64(int32(5)), wasm.ValueTypeI32)
	assertReturn3("as-select-second", uint64(int32(0)), uint64(int32(0)), uint64(int32(6)), wasm.ValueTypeI32)
	assertReturn2("as-select-cond", uint64(int32(0)), uint64(int32(0)), wasm.ValueTypeI32)

	assertReturn2("as-call-first", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)
	assertReturn2("as-call-mid", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)
	assertReturn2("as-call-last", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)

	assertReturn2("as-call_indirect-first", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)
	assertReturn2("as-call_indirect-mid", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)
	assertReturn2("as-call_indirect-last", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)
	assertReturn2("as-call_indirect-index", uint64(int32(0)), uint64(uint32(0xffffffff)), wasm.ValueTypeI32)

	assertReturnWithoutReturn("as-local.set-value")
	assertReturn2("as-local.tee-value", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)
	assertReturnWithoutReturn("as-global.set-value")

	assertReturn2("as-load-address", uint64(int32(0)), uint64(int32(0)), wasm.ValueTypeI32)
	assertReturn2("as-loadN-address", uint64(int32(0)), uint64(int32(0)), wasm.ValueTypeI32)
	assertReturnWithoutReturn("as-store-address", uint64(int32(0)))
	assertReturnWithoutReturn("as-store-value", uint64(int32(0)))
	assertReturnWithoutReturn("as-storeN-address", uint64(int32(0)))
	assertReturnWithoutReturn("as-storeN-value", uint64(int32(0)))

	assertReturn2("as-unary-operand", uint64(math.Float32bits(0)), uint64(math.Float32bits(float32(math.NaN()))), wasm.ValueTypeF32)
	assertReturn2("as-binary-left", uint64(int32(0)), uint64(int32(13)), wasm.ValueTypeI32)
	assertReturn2("as-binary-right", uint64(int32(0)), uint64(int32(6)), wasm.ValueTypeI32)
	assertReturn2("as-test-operand", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)
	assertReturn2("as-compare-left", uint64(int32(0)), uint64(int32(0)), wasm.ValueTypeI32)
	assertReturn2("as-compare-right", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)
	assertReturn2("as-convert-operand", uint64(int64(0)), uint64(int32(41)), wasm.ValueTypeI32)
	assertReturn2("as-memory.grow-size", uint64(int32(0)), uint64(int32(1)), wasm.ValueTypeI32)

	assertReturnWithoutReturn("type-mixed", uint64(int64(1)), uint64(math.Float32bits(2.2)), math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)))

	assertReturn4("write", uint64(int64(1)), uint64(math.Float32bits(2)), math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)), uint64(int64(56)), wasm.ValueTypeI64)

	assertReturn4("result", 0xffffffffffffffff, uint64(math.Float32bits(-2)), math.Float64bits(-3.3), uint64(uint32(0xfffffffc)), uint64(uint32(0xfffffffb)), math.Float64bits(34.8), wasm.ValueTypeF64)
}
