package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_load(t *testing.T) {
	vm := requireInitVM(t, "load", nil)

	assertReturn := func(name string, expReturn bool, vars ...uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			if expReturn {
				values, types, err := vm.ExecExportedFunction(name, vars[:len(vars)-1]...)
				require.NoError(t, err)
				require.Len(t, values, 1)
				require.Len(t, types, 1)
				require.Equal(t, types[0], wasm.ValueTypeI32)
				require.Equal(t, uint32(vars[len(vars)-1]), uint32(values[0]))
			} else {
				values, types, err := vm.ExecExportedFunction(name, vars...)
				require.NoError(t, err)
				require.Len(t, values, 0)
				require.Len(t, types, 0)
			}
		})
	}

	assertReturn("as-br-value", true, 0)

	assertReturn("as-br_if-cond", false)
	assertReturn("as-br_if-value", true, 0)
	assertReturn("as-br_if-value-cond", true, 7)

	assertReturn("as-br_table-index", false)
	assertReturn("as-br_table-value", true, 0)
	assertReturn("as-br_table-value-index", true, 6)

	assertReturn("as-return-value", true, 0)

	assertReturn("as-if-cond", true, 1)
	assertReturn("as-if-then", true, 0)
	assertReturn("as-if-else", true, 0)

	assertReturn("as-select-first", true, 0, 1, 0)
	assertReturn("as-select-second", true, 0, 0, 0)
	assertReturn("as-select-cond", true, 1)

	assertReturn("as-call-first", true, uint64(uint32(0xffffffff)))
	assertReturn("as-call-mid", true, uint64(uint32(0xffffffff)))
	assertReturn("as-call-last", true, uint64(uint32(0xffffffff)))

	assertReturn("as-call_indirect-first", true, uint64(uint32(0xffffffff)))
	assertReturn("as-call_indirect-mid", true, uint64(uint32(0xffffffff)))
	assertReturn("as-call_indirect-last", true, uint64(uint32(0xffffffff)))
	assertReturn("as-call_indirect-index", true, uint64(uint32(0xffffffff)))

	assertReturn("as-local.set-value", false)
	assertReturn("as-local.tee-value", true, 0)
	assertReturn("as-global.set-value", false)

	assertReturn("as-load-address", true, 0)
	assertReturn("as-loadN-address", true, 0)
	assertReturn("as-store-address", false)
	assertReturn("as-store-value", false)
	assertReturn("as-storeN-address", false)
	assertReturn("as-storeN-value", false)

	assertReturn("as-unary-operand", true, 32)

	assertReturn("as-binary-left", true, 10)
	assertReturn("as-binary-right", true, 10)

	assertReturn("as-test-operand", true, 1)

	assertReturn("as-compare-left", true, 1)
	assertReturn("as-compare-right", true, 1)

	assertReturn("as-memory.grow-size", true, 1)

}
