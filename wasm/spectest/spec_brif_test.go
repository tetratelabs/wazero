package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_brif(t *testing.T) {
	vm := requireInitVM(t, "brif", nil)

	assertReturnExecNonReturnFunction := func(name string, args ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, args...)
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

	assertReturnExecReturnFunction("type-i32-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("type-i64-value", 2, wasm.ValueTypeI64)
	assertReturnExecReturnFunction("type-f32-value", uint64(math.Float32bits(3.0)), wasm.ValueTypeF32)
	assertReturnExecReturnFunction("type-f64-value", math.Float64bits(4.0), wasm.ValueTypeF64)

	assertReturnExecReturnFunction("as-block-first", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-block-first", 3, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-block-mid", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-block-mid", 3, wasm.ValueTypeI32, 1)
	assertReturnExecNonReturnFunction("as-block-last", 0)
	assertReturnExecNonReturnFunction("as-block-last", 1)

	assertReturnExecReturnFunction("as-block-first-value", 11, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-block-first-value", 10, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-block-mid-value", 21, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-block-mid-value", 20, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-block-last-value", 11, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-block-last-value", 11, wasm.ValueTypeI32, 1)

	assertReturnExecReturnFunction("as-loop-first", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-loop-first", 3, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-loop-mid", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-loop-mid", 4, wasm.ValueTypeI32, 1)
	assertReturnExecNonReturnFunction("as-loop-last", 0)
	assertReturnExecNonReturnFunction("as-loop-last", 1)

	assertReturnExecReturnFunction("as-br-value", 1, wasm.ValueTypeI32)

	assertReturnExecNonReturnFunction("as-br_if-cond")
	assertReturnExecReturnFunction("as-br_if-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_if-value-cond", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-br_if-value-cond", 1, wasm.ValueTypeI32, 1)

	assertReturnExecNonReturnFunction("as-br_table-index")
	assertReturnExecReturnFunction("as-br_table-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_table-value-index", 1, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-return-value", 1, wasm.ValueTypeI64)

	assertReturnExecReturnFunction("as-if-cond", 2, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-if-cond", 1, wasm.ValueTypeI32, 1)
	assertReturnExecNonReturnFunction("as-if-then", 0, 0)
	assertReturnExecNonReturnFunction("as-if-then", 4, 0)
	assertReturnExecNonReturnFunction("as-if-then", 0, 1)
	assertReturnExecNonReturnFunction("as-if-then", 4, 1)
	assertReturnExecNonReturnFunction("as-if-else", 0, 0)
	assertReturnExecNonReturnFunction("as-if-else", 3, 0)
	assertReturnExecNonReturnFunction("as-if-else", 0, 1)
	assertReturnExecNonReturnFunction("as-if-else", 3, 1)

	assertReturnExecReturnFunction("as-select-first", 3, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-select-first", 3, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-select-second", 3, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-select-second", 3, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("as-select-cond", 3, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-call-first", 12, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-mid", 13, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-last", 14, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-call_indirect-func", 4, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-first", 4, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-mid", 4, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-last", 4, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-local.set-value", 0xffffffffffffffff, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-local.set-value", 17, wasm.ValueTypeI32, 1)

	assertReturnExecReturnFunction("as-local.tee-value", 0xffffffffffffffff, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-local.tee-value", 1, wasm.ValueTypeI32, 1)

	assertReturnExecReturnFunction("as-global.set-value", 0xffffffffffffffff, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("as-global.set-value", 1, wasm.ValueTypeI32, 1)

	assertReturnExecReturnFunction("as-load-address", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-loadN-address", 30, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-store-address", 30, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-store-value", 31, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-storeN-address", 32, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-storeN-value", 33, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-unary-operand", math.Float64bits(1.0), wasm.ValueTypeF64)
	assertReturnExecReturnFunction("as-binary-left", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-binary-right", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-test-operand", 0, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-left", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-right", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-memory.grow-size", 1, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("nested-block-value", 21, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-block-value", 9, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("nested-br-value", 5, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-br-value", 9, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("nested-br_if-value", 5, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-br_if-value", 9, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("nested-br_if-value-cond", 5, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-br_if-value-cond", 9, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("nested-br_table-value", 5, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-br_table-value", 9, wasm.ValueTypeI32, 1)
	assertReturnExecReturnFunction("nested-br_table-value-index", 5, wasm.ValueTypeI32, 0)
	assertReturnExecReturnFunction("nested-br_table-value-index", 9, wasm.ValueTypeI32, 1)
}
