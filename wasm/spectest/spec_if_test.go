package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_if(t *testing.T) {
	vm := requireInitVM(t, "if", nil)

	assertReturn := func(name string) {
		values, types, err := vm.ExecExportedFunction(name)
		require.NoError(t, err)
		require.Len(t, values, 0)
		require.Len(t, types, 0)
	}

	// ;; \(assert_return\s\(invoke "([a-z.A-Z-_]+)"\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn__I32("$1", $2)
	assertReturn__I32 := func(name string, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	// ;; \(assert_return\s\(invoke "([a-zA-Z-_]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\)
	// assertReturn_I32("$1", $2)
	assertReturn_I32 := func(name string, arg uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 0)
			require.Len(t, types, 0)
		})
	}
	// ;; \(assert_return\s\(invoke "([a-z.A-Z-_]+)"\s\(i32.const\s([a-z0-9-]+)\)\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I32("$1", $2, $3)
	assertReturn_I32_I32 := func(name string, arg, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, exp, uint32(values[0]))
		})
	}
	// ;; \(assert_return\s\(invoke "([a-zA-Z_-]+)"\s\(i32.const\s([a-z0-9-]+)\) \(i32.const\s([a-z0-9-]+)\)\)\s\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32I32_I32("$1", $2, $3, $4)
	assertReturn_I32I32_I32 := func(name string, arg1, arg2, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg1), uint64(arg2))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, exp, uint32(values[0]))
		})
	}

	assertTrap := func(name string, arg uint32) {
		t.Run("assert_trap_"+name, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(name, uint64(arg))
			})
		})
	}

	assertReturn_I32("empty", 0)
	assertReturn_I32("empty", 1)
	assertReturn_I32("empty", 100)
	assertReturn_I32("empty", 0xfffffffe)

	assertReturn_I32_I32("singular", 0, 8)
	assertReturn_I32_I32("singular", 1, 7)
	assertReturn_I32_I32("singular", 10, 7)
	assertReturn_I32_I32("singular", 0xfffffff6, 7)

	assertReturn_I32I32_I32("nested", 0, 0, 11)
	assertReturn_I32I32_I32("nested", 1, 0, 10)
	assertReturn_I32I32_I32("nested", 0, 1, 10)
	assertReturn_I32I32_I32("nested", 3, 2, 9)
	assertReturn_I32I32_I32("nested", 0, 0xffffff9c, 10)
	assertReturn_I32I32_I32("nested", 10, 10, 9)
	assertReturn_I32I32_I32("nested", 0, 0xffffffff, 10)
	assertReturn_I32I32_I32("nested", 0xffffff91, 0xfffffffe, 9)

	assertReturn_I32_I32("as-select-first", 0, 0)
	assertReturn_I32_I32("as-select-first", 1, 1)
	assertReturn_I32_I32("as-select-mid", 0, 2)
	assertReturn_I32_I32("as-select-mid", 1, 2)
	assertReturn_I32_I32("as-select-last", 0, 3)
	assertReturn_I32_I32("as-select-last", 1, 2)

	assertReturn_I32_I32("as-loop-first", 0, 0)
	assertReturn_I32_I32("as-loop-first", 1, 1)
	assertReturn_I32_I32("as-loop-mid", 0, 0)
	assertReturn_I32_I32("as-loop-mid", 1, 1)
	assertReturn_I32_I32("as-loop-last", 0, 0)
	assertReturn_I32_I32("as-loop-last", 1, 1)

	assertReturn_I32_I32("as-if-condition", 0, 3)
	assertReturn_I32_I32("as-if-condition", 1, 2)

	assertReturn_I32_I32("as-br_if-first", 0, 0)
	assertReturn_I32_I32("as-br_if-first", 1, 1)
	assertReturn_I32_I32("as-br_if-last", 0, 3)
	assertReturn_I32_I32("as-br_if-last", 1, 2)

	assertReturn_I32_I32("as-br_table-first", 0, 0)
	assertReturn_I32_I32("as-br_table-first", 1, 1)
	assertReturn_I32_I32("as-br_table-last", 0, 2)
	assertReturn_I32_I32("as-br_table-last", 1, 2)

	assertReturn_I32_I32("as-call_indirect-first", 0, 0)
	assertReturn_I32_I32("as-call_indirect-first", 1, 1)
	assertReturn_I32_I32("as-call_indirect-mid", 0, 2)
	assertReturn_I32_I32("as-call_indirect-mid", 1, 2)
	assertReturn_I32_I32("as-call_indirect-last", 0, 2)
	assertTrap("as-call_indirect-last", 1)

	assertReturn_I32("as-store-first", 0)
	assertReturn_I32("as-store-first", 1)
	assertReturn_I32("as-store-last", 0)
	assertReturn_I32("as-store-last", 1)

	assertReturn_I32_I32("as-memory.grow-value", 0, 1)
	assertReturn_I32_I32("as-memory.grow-value", 1, 1)

	assertReturn_I32_I32("as-call-value", 0, 0)
	assertReturn_I32_I32("as-call-value", 1, 1)

	assertReturn_I32_I32("as-return-value", 0, 0)
	assertReturn_I32_I32("as-return-value", 1, 1)

	assertReturn_I32("as-drop-operand", 0)
	assertReturn_I32("as-drop-operand", 1)

	assertReturn_I32_I32("as-br-value", 0, 0)
	assertReturn_I32_I32("as-br-value", 1, 1)

	assertReturn_I32_I32("as-local.set-value", 0, 0)
	assertReturn_I32_I32("as-local.set-value", 1, 1)

	assertReturn_I32_I32("as-local.tee-value", 0, 0)
	assertReturn_I32_I32("as-local.tee-value", 1, 1)

	assertReturn_I32_I32("as-global.set-value", 0, 0)
	assertReturn_I32_I32("as-global.set-value", 1, 1)

	assertReturn_I32_I32("as-load-operand", 0, 0)
	assertReturn_I32_I32("as-load-operand", 1, 0)

	assertReturn_I32_I32("as-unary-operand", 0, 0)
	assertReturn_I32_I32("as-unary-operand", 1, 0)
	assertReturn_I32_I32("as-unary-operand", 0xffffffff, 0)

	assertReturn_I32I32_I32("as-binary-operand", 0, 0, 15)
	assertReturn_I32I32_I32("as-binary-operand", 0, 1, 0xfffffff4)
	assertReturn_I32I32_I32("as-binary-operand", 1, 0, 0xfffffff1)
	assertReturn_I32I32_I32("as-binary-operand", 1, 1, 12)

	assertReturn_I32_I32("as-test-operand", 0, 1)
	assertReturn_I32_I32("as-test-operand", 1, 0)

	assertReturn_I32I32_I32("as-compare-operand", 0, 0, 1)
	assertReturn_I32I32_I32("as-compare-operand", 0, 1, 0)
	assertReturn_I32I32_I32("as-compare-operand", 1, 0, 1)
	assertReturn_I32I32_I32("as-compare-operand", 1, 1, 0)

	assertReturn_I32_I32("as-binary-operands", 0, 0xfffffff4)
	assertReturn_I32_I32("as-binary-operands", 1, 12)

	assertReturn_I32_I32("as-compare-operands", 0, 1)
	assertReturn_I32_I32("as-compare-operands", 1, 0)

	assertReturn_I32_I32("as-mixed-operands", 0, 0xfffffffd)
	assertReturn_I32_I32("as-mixed-operands", 1, 27)

	assertReturn__I32("break-bare", 19)
	assertReturn_I32_I32("break-value", 1, 18)
	assertReturn_I32_I32("break-value", 0, 21)

	assertReturn_I32_I32("param", 0, 0xffffffff)
	assertReturn_I32_I32("param", 1, 3)
	assertReturn_I32_I32("params", 0, 0xffffffff)
	assertReturn_I32_I32("params", 1, 3)
	assertReturn_I32_I32("params-id", 0, 3)
	assertReturn_I32_I32("params-id", 1, 3)
	assertReturn_I32_I32("param-break", 0, 0xffffffff)
	assertReturn_I32_I32("param-break", 1, 3)
	assertReturn_I32_I32("params-break", 0, 0xffffffff)
	assertReturn_I32_I32("params-break", 1, 3)
	assertReturn_I32_I32("params-id-break", 0, 3)
	assertReturn_I32_I32("params-id-break", 1, 3)

	assertReturn_I32_I32("effects", 1, 0xfffffff2)
	assertReturn_I32_I32("effects", 0, 0xfffffffa)
	assertReturn("type-use")
}
