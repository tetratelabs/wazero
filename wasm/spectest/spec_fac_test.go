package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_fac(t *testing.T) {
	vm := requireInitVM(t, "fac", nil)

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

	assertReturn_I64_I64("fac-rec", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-iter", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-rec-named", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-iter-named", 25, 7034535277573963776)
	assertReturn_I64_I64("fac-opt", 25, 7034535277573963776)
	// FIXME; needs multivalue return support.
	// assertReturn_I64_I64("fac-ssa", 25, 3628800)
}
