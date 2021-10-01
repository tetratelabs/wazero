package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_br(t *testing.T) {
	vm := requireInitVM(t, "br", nil)

	assertReturnExecNonReturnFunction := func(name string) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}
	assertReturnExecReturnFunction := func(name string, exp uint64, expType wasm.ValueType, args ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, args...)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], expType)
			require.Equal(t, exp, values[0])
		})
	}

	assertReturnExecNonReturnFunction("type-i32")
	assertReturnExecNonReturnFunction("type-i64")
	assertReturnExecNonReturnFunction("type-f32")
	assertReturnExecNonReturnFunction("type-f64")
	assertReturnExecNonReturnFunction("type-i32-i32")
	assertReturnExecNonReturnFunction("type-i64-i64")
	assertReturnExecNonReturnFunction("type-f32-f32")
	assertReturnExecNonReturnFunction("type-f64-f64")

	assertReturnExecReturnFunction("type-i32-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("type-i64-value", 2, wasm.ValueTypeI64)
	assertReturnExecReturnFunction("type-f32-value", uint64(math.Float32bits(3.0)), wasm.ValueTypeF32)
	assertReturnExecReturnFunction("type-f64-value", math.Float64bits(4.0), wasm.ValueTypeF64)
	// FIXME after muiltiple value returnes
	// ;; (assert_return (invoke "type-f64-f64-value") (f64.const 4) (f64.const 5))

	assertReturnExecNonReturnFunction("as-block-first")
	assertReturnExecNonReturnFunction("as-block-mid")
	assertReturnExecNonReturnFunction("as-block-last")
	assertReturnExecReturnFunction("as-block-value", 2, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-loop-first", 3, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-loop-mid", 4, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-loop-last", 5, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-br-value", 9, wasm.ValueTypeI32)

	assertReturnExecNonReturnFunction("as-br_if-cond")
	assertReturnExecReturnFunction("as-br_if-value", 8, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_if-value-cond", 9, wasm.ValueTypeI32)

	assertReturnExecNonReturnFunction("as-br_table-index")
	assertReturnExecReturnFunction("as-br_table-value", 10, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_table-value-index", 11, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-return-value", 7, wasm.ValueTypeI64)
	// FIXME after muiltiple value returnes
	// ;; (assert_return (invoke "as-return-values") (i32.const 2) (i64.const 7))

	assertReturnExecReturnFunction("as-if-cond", 2, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-if-then", 3, wasm.ValueTypeI32, 1, 6)
	assertReturnExecReturnFunction("as-if-then", 6, wasm.ValueTypeI32, 0, 6)
	assertReturnExecReturnFunction("as-if-else", 4, wasm.ValueTypeI32, 0, 6)
	assertReturnExecReturnFunction("as-if-else", 6, wasm.ValueTypeI32, 1, 6)

	assertReturnExecReturnFunction("as-select-first", 5, wasm.ValueTypeI32, 0, 6)
	assertReturnExecReturnFunction("as-select-first", 5, wasm.ValueTypeI32, 1, 6)
	assertReturnExecReturnFunction("as-select-second", 6, wasm.ValueTypeI32, 0, 6)
	assertReturnExecReturnFunction("as-select-second", 6, wasm.ValueTypeI32, 1, 6)
	assertReturnExecReturnFunction("as-select-cond", 7, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-select-all", 8, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-call-first", 12, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-mid", 13, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-last", 14, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-all", 15, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-call_indirect-func", 20, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-first", 21, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-mid", 22, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-last", 23, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-all", 24, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-local.set-value", 17, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-local.tee-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-global.set-value", 1, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-load-address", uint64(math.Float32bits(1.7)), wasm.ValueTypeF32)
	assertReturnExecReturnFunction("as-loadN-address", 30, wasm.ValueTypeI64)

	assertReturnExecReturnFunction("as-store-address", 30, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-store-value", 31, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-store-both", 32, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-storeN-address", 32, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-storeN-value", 33, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-storeN-both", 34, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-unary-operand", uint64(math.Float32bits(3.4)), wasm.ValueTypeF32)

	assertReturnExecReturnFunction("as-binary-left", 3, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-binary-right", 45, wasm.ValueTypeI64)
	assertReturnExecReturnFunction("as-binary-both", 46, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-test-operand", 44, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-compare-left", 43, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-right", 42, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-both", 44, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-convert-operand", 41, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-memory.grow-size", 40, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("nested-block-value", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested-br-value", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested-br_if-value", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested-br_if-value-cond", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested-br_table-value", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested-br_table-value-index", 9, wasm.ValueTypeI32)
}
