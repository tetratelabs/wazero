package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_local_set(t *testing.T) {
	vm := requireInitVM(t, "local_set", nil)

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

	assertReturnWithoutReturn("type-local-i32")
	assertReturnWithoutReturn("type-local-i64")
	assertReturnWithoutReturn("type-local-f32")
	assertReturnWithoutReturn("type-local-f64")

	assertReturnWithoutReturn("type-param-i32", uint64(int32(2)))
	assertReturnWithoutReturn("type-param-i64", uint64(int64(3)))
	assertReturnWithoutReturn("type-param-f32", uint64(math.Float64bits(4.4)))
	assertReturnWithoutReturn("type-param-f64", math.Float64bits(5.5))

	assertReturnWithoutReturn("as-block-value", 0)
	assertReturnWithoutReturn("as-loop-value", 0)

	assertReturnWithoutReturn("as-br-value", 0)
	assertReturnWithoutReturn("as-br_if-value", 0)
	assertReturnWithoutReturn("as-br_if-value-cond", 0)
	assertReturnWithoutReturn("as-br_table-value", 0)

	assertReturnWithoutReturn("as-return-value", 0)

	assertReturnWithoutReturn("as-if-then", 1)
	assertReturnWithoutReturn("as-if-else", 0)

	assertReturnWithoutReturn("type-mixed",
		uint64(int64(1)), uint64(math.Float32bits(2.2)),
		math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)),
	)

	assertReturn("write",
		uint64(int64(56)), wasm.ValueTypeI64,
		uint64(int64(1)), uint64(math.Float32bits(2)),
		math.Float64bits(3.3), uint64(int32(4)), uint64(int32(5)),
	)
}
