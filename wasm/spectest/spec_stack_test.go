package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_stack(t *testing.T) {
	vm := requireInitVM(t, "stack", nil)

	assertReturn_I64_I64 := func(name string, arg, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, arg)
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI64, types[0])
			require.Equal(t, exp, values[0])

		})
	}

	// \(assert_return\s\(invoke "([a-z.A-Z-_0-9]+)"\)\s\(i32.const\s([a-z0-9-]+)\)\)
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

	assertReturn_I64_I64("fac-expr", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-stack", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-mixed", 25, 7034535277573963776)

	assertReturn__I32("not-quite-a-tree", 3)
	assertReturn__I32("not-quite-a-tree", 9)
}
