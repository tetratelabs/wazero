package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_local_get(t *testing.T) {
	vm := requireInitVM(t, "local_get", nil)

	assertReturn := func(name string, exp uint64, expType wasm.ValueType, args ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, args...)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, expType, types[0])
			require.Equal(t, exp, values[0])
		})
	}

	assertReturnWithoutReturn := func(name string, arg ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg...)
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}

	assertReturn("type-local-i32", 0, wasm.ValueTypeI32)
	assertReturn("type-local-i64", 0, wasm.ValueTypeI64)
	assertReturn("type-local-f32", 0, wasm.ValueTypeF32)
	assertReturn("type-local-f64", 0, wasm.ValueTypeF64)

	assertReturn("type-param-i32", uint64(int32(2)), wasm.ValueTypeI32, uint64(int32(2)))
	assertReturn("type-param-i64", uint64(int64(3)), wasm.ValueTypeI64, uint64(int64(3)))
	assertReturn("type-param-f32", uint64(math.Float32bits(4.4)), wasm.ValueTypeF32, uint64(math.Float32bits(4.4)))
	assertReturn("type-param-f64", math.Float64bits(5.5), wasm.ValueTypeF64, math.Float64bits(5.5))

	assertReturn("as-block-value", uint64(int32(6)), wasm.ValueTypeI32, uint64(int32(6)))
	assertReturn("as-loop-value", uint64(int32(7)), wasm.ValueTypeI32, uint64(int32(7)))

	assertReturn("as-br-value", uint64(int32(8)), wasm.ValueTypeI32, uint64(int32(8)))
	assertReturn("as-br_if-value", uint64(int32(9)), wasm.ValueTypeI32, uint64(int32(9)))
	assertReturn("as-br_if-value-cond", uint64(int32(10)), wasm.ValueTypeI32, uint64(int32(10)))
	assertReturn("as-br_table-value", uint64(int32(2)), wasm.ValueTypeI32, uint64(int32(1)))

	assertReturn("as-return-value", 0, wasm.ValueTypeI32, 0)

	assertReturn("as-if-then", 1, wasm.ValueTypeI32, 1)
	assertReturn("as-if-else", 0, wasm.ValueTypeI32, 0)

	assertReturnWithoutReturn("type-mixed",
		uint64(int64(1)), uint64(math.Float32bits(2.2)), math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)),
	)

	assertReturn("read", math.Float64bits(34.8), wasm.ValueTypeF64,
		uint64(int64(1)), uint64(math.Float32bits(2)), math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)))
}
