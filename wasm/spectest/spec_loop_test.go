package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_loop(t *testing.T) {
	vm := requireInitVM(t, "loop", nil)

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

	assertReturnExecNonReturnFunction("empty")
	assertReturnExecReturnFunction("singular", 7, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("multi", 8, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("nested", 9, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("deep", 150, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-select-first", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-select-mid", 2, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-select-last", 2, wasm.ValueTypeI32)

	assertReturnExecNonReturnFunction("as-if-condition")
	assertReturnExecReturnFunction("as-if-then", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-if-else", 2, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-br_if-first", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_if-last", 2, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-br_table-first", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-br_table-last", 2, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-first", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-mid", 2, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call_indirect-last", 1, wasm.ValueTypeI32)

	assertReturnExecNonReturnFunction("as-store-first")
	assertReturnExecNonReturnFunction("as-store-last")

	assertReturnExecReturnFunction("as-memory.grow-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-call-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-return-value", 1, wasm.ValueTypeI32)
	assertReturnExecNonReturnFunction("as-drop-operand")
	assertReturnExecReturnFunction("as-br-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-local.set-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-local.tee-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-global.set-value", 1, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-load-operand", 1, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("as-unary-operand", 0, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-binary-operand", 12, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-test-operand", 0, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-operand", 0, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-binary-operands", 12, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-compare-operands", 0, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("as-mixed-operands", 27, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("break-bare", 19, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("break-value", 18, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("break-repeated", 18, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("break-inner", 0x1f, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("param", 3, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("params", 3, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("params-id", 3, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("param-break", 13, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("params-break", 12, wasm.ValueTypeI32)
	assertReturnExecReturnFunction("params-id-break", 3, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("effects", 1, wasm.ValueTypeI32)

	assertReturnExecReturnFunction("while", 1, wasm.ValueTypeI64, 0)
	assertReturnExecReturnFunction("while", 1, wasm.ValueTypeI64, 1)
	assertReturnExecReturnFunction("while", 2, wasm.ValueTypeI64, 2)
	assertReturnExecReturnFunction("while", 6, wasm.ValueTypeI64, 3)
	assertReturnExecReturnFunction("while", 120, wasm.ValueTypeI64, 5)
	assertReturnExecReturnFunction("while", 2432902008176640000, wasm.ValueTypeI64, 20)

	assertReturnExecReturnFunction("for", 1, wasm.ValueTypeI64, 0)
	assertReturnExecReturnFunction("for", 1, wasm.ValueTypeI64, 1)
	assertReturnExecReturnFunction("for", 2, wasm.ValueTypeI64, 2)
	assertReturnExecReturnFunction("for", 6, wasm.ValueTypeI64, 3)
	assertReturnExecReturnFunction("for", 120, wasm.ValueTypeI64, 5)
	assertReturnExecReturnFunction("for", 2432902008176640000, wasm.ValueTypeI64, 20)

	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(0)), wasm.ValueTypeF32, uint64(math.Float32bits(0)), uint64(math.Float32bits(7)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(0)), wasm.ValueTypeF32, uint64(math.Float32bits(7)), uint64(math.Float32bits(0)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(1)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(1)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(2)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(2)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(4)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(3)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(6)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(4)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(2550)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(100)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(2601)), wasm.ValueTypeF32, uint64(math.Float32bits(1)), uint64(math.Float32bits(101)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(1)), wasm.ValueTypeF32, uint64(math.Float32bits(2)), uint64(math.Float32bits(1)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(1)), wasm.ValueTypeF32, uint64(math.Float32bits(3)), uint64(math.Float32bits(1)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(1)), wasm.ValueTypeF32, uint64(math.Float32bits(10)), uint64(math.Float32bits(1)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(3)), wasm.ValueTypeF32, uint64(math.Float32bits(2)), uint64(math.Float32bits(2)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(4)), wasm.ValueTypeF32, uint64(math.Float32bits(2)), uint64(math.Float32bits(3)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(10.3095235825)), wasm.ValueTypeF32, uint64(math.Float32bits(7)), uint64(math.Float32bits(4)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(4381.54785156)), wasm.ValueTypeF32, uint64(math.Float32bits(7)), uint64(math.Float32bits(100)))
	assertReturnExecReturnFunction("nesting", uint64(math.Float32bits(2601)), wasm.ValueTypeF32, uint64(math.Float32bits(7)), uint64(math.Float32bits(101)))

	assertReturnExecNonReturnFunction("type-use")
}
