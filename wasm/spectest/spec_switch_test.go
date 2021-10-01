package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_switch(t *testing.T) {
	vm := requireInitVM(t, "switch", nil)

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)[\s\n]+\(i32.const\s([a-z0-9-]+)\)\)
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

	// \(assert_return[\s\n]+\(invoke "([a-zA-Z-_]+)"[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)[\s\n]+\(i64.const\s([a-z0-9-]+)\)\)
	// assertReturn_I64_I64("$1", $2, $3)
	assertReturn_I64_I64 := func(name string, arg, exp uint64) {
		t.Run("assert_return_"+name, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(name, uint64(arg))
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

	assertReturn_I32_I32("stmt", 0, 0)
	assertReturn_I32_I32("stmt", 1, 0xffffffff)
	assertReturn_I32_I32("stmt", 2, 0xfffffffe)
	assertReturn_I32_I32("stmt", 3, 0xfffffffd)
	assertReturn_I32_I32("stmt", 4, 100)
	assertReturn_I32_I32("stmt", 5, 101)
	assertReturn_I32_I32("stmt", 6, 102)
	assertReturn_I32_I32("stmt", 7, 100)
	assertReturn_I32_I32("stmt", 0xfffffff6, 102)

	assertReturn_I64_I64("expr", 0, 0)
	assertReturn_I64_I64("expr", 1, 0xffffffffffffffff)
	assertReturn_I64_I64("expr", 2, 0xfffffffffffffffe)
	assertReturn_I64_I64("expr", 3, 0xfffffffffffffffd)
	assertReturn_I64_I64("expr", 6, 101)
	assertReturn_I64_I64("expr", 7, 0xfffffffffffffffb)
	assertReturn_I64_I64("expr", 0xfffffffffffffff6, 100)

	assertReturn_I32_I32("arg", 0, 110)
	assertReturn_I32_I32("arg", 1, 12)
	assertReturn_I32_I32("arg", 2, 4)
	assertReturn_I32_I32("arg", 3, 1116)
	assertReturn_I32_I32("arg", 4, 118)
	assertReturn_I32_I32("arg", 5, 20)
	assertReturn_I32_I32("arg", 6, 12)
	assertReturn_I32_I32("arg", 7, 1124)
	assertReturn_I32_I32("arg", 8, 126)

	assertReturn__I32("corner", 1)
}
