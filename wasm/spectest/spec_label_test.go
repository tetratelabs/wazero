package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_label(t *testing.T) {
	vm := requireInitVM(t, "label", nil)

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-0-9_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
	// assertReturn_I32_I32("$1", $2, $3)
	assertReturn_I32_I32 := func(name string, arg, exp uint32) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, wasm.ValueTypeI32, types[0])
			require.Equal(t, exp, uint32(values[0]))

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

	assertReturn__I32("block", 1)
	assertReturn__I32("loop1", 5)
	assertReturn__I32("loop2", 8)
	assertReturn__I32("loop3", 1)
	assertReturn_I32_I32("loop4", 8, 16)
	assertReturn__I32("loop5", 2)
	assertReturn__I32("loop6", 3)
	assertReturn__I32("if", 5)
	assertReturn__I32("if2", 5)
	assertReturn_I32_I32("switch", 0, 50)
	assertReturn_I32_I32("switch", 1, 20)
	assertReturn_I32_I32("switch", 2, 20)
	assertReturn_I32_I32("switch", 3, 3)
	assertReturn_I32_I32("switch", 4, 50)
	assertReturn_I32_I32("switch", 5, 50)
	assertReturn_I32_I32("return", 0, 0)
	assertReturn_I32_I32("return", 1, 2)
	assertReturn_I32_I32("return", 2, 2)
	assertReturn__I32("br_if0", 0x1d)
	assertReturn__I32("br_if1", 1)
	assertReturn__I32("br_if2", 1)
	assertReturn__I32("br_if3", 2)
	assertReturn__I32("br", 1)
	assertReturn__I32("shadowing", 1)
	assertReturn__I32("redefinition", 5)
}
