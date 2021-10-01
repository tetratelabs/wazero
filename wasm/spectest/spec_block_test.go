package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_block(t *testing.T) {
	vm := requireInitVM(t, "block", nil)

	assertReturnExecNonReturnFunction := func(name string) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}
	assertReturnExecReturnFunction := func(name string, exp int32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], wasm.ValueTypeI32)
			require.Equal(t, exp, int32(uint32(values[0])))
		})
	}

	assertReturnExecNonReturnFunction("empty")
	assertReturnExecReturnFunction("singular", 7)
	assertReturnExecReturnFunction("multi", 8)
	assertReturnExecReturnFunction("nested", 9)
	assertReturnExecReturnFunction("deep", 150)

	assertReturnExecReturnFunction("as-select-first", 1)
	assertReturnExecReturnFunction("as-select-mid", 2)
	assertReturnExecReturnFunction("as-select-last", 2)

	assertReturnExecReturnFunction("as-loop-first", 1)
	assertReturnExecReturnFunction("as-loop-mid", 1)
	assertReturnExecReturnFunction("as-loop-last", 1)

	assertReturnExecNonReturnFunction("as-if-condition")
	assertReturnExecReturnFunction("as-if-then", 1)
	assertReturnExecReturnFunction("as-if-else", 2)

	assertReturnExecReturnFunction("as-br_if-first", 1)
	assertReturnExecReturnFunction("as-br_if-last", 2)

	assertReturnExecReturnFunction("as-br_table-first", 1)
	assertReturnExecReturnFunction("as-br_table-last", 2)

	assertReturnExecReturnFunction("as-call_indirect-first", 1)
	assertReturnExecReturnFunction("as-call_indirect-mid", 2)
	assertReturnExecReturnFunction("as-call_indirect-last", 1)

	assertReturnExecNonReturnFunction("as-store-first")
	assertReturnExecNonReturnFunction("as-store-last")

	assertReturnExecReturnFunction("as-memory.grow-value", 1)
	assertReturnExecReturnFunction("as-call-value", 1)
	assertReturnExecReturnFunction("as-return-value", 1)
	assertReturnExecNonReturnFunction("as-drop-operand")
	assertReturnExecReturnFunction("as-br-value", 1)
	assertReturnExecReturnFunction("as-local.set-value", 1)
	assertReturnExecReturnFunction("as-local.tee-value", 1)
	assertReturnExecReturnFunction("as-global.set-value", 1)
	assertReturnExecReturnFunction("as-load-operand", 1)

	assertReturnExecReturnFunction("as-unary-operand", 0)
	assertReturnExecReturnFunction("as-binary-operand", 12)
	assertReturnExecReturnFunction("as-test-operand", 0)
	assertReturnExecReturnFunction("as-compare-operand", 0)
	assertReturnExecReturnFunction("as-binary-operands", 12)
	assertReturnExecReturnFunction("as-compare-operands", 0)
	assertReturnExecReturnFunction("as-mixed-operands", 27)

	assertReturnExecReturnFunction("break-bare", 19)
	assertReturnExecReturnFunction("break-value", 18)
	// FIXME after multi value support
	// ;; (assert_return (invoke "break-multi-value")
	// ;;   (i32.const 18) (i32.const -18) (i64.const 18)
	// ;; )
	assertReturnExecReturnFunction("break-repeated", 18)
	assertReturnExecReturnFunction("break-inner", 0xf)

	assertReturnExecReturnFunction("param", 3)
	assertReturnExecReturnFunction("params", 3)
	assertReturnExecReturnFunction("params-id", 3)
	assertReturnExecReturnFunction("param-break", 3)
	assertReturnExecReturnFunction("params-break", 3)
	assertReturnExecReturnFunction("params-id-break", 3)

	assertReturnExecReturnFunction("effects", 1)

	assertReturnExecNonReturnFunction("type-use")
}
