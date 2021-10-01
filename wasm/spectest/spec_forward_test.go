package spectest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_forward(t *testing.T) {
	vm := requireInitVM(t, "forward", nil)

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

	assertReturn_I32_I32("even", 13, 0)
	assertReturn_I32_I32("even", 20, 1)
	assertReturn_I32_I32("odd", 13, 1)
	assertReturn_I32_I32("odd", 20, 0)
}
