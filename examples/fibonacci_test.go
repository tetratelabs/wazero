package examples

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func Test_fibonacci(t *testing.T) {
	buf, err := os.ReadFile("wasm/fibonacci.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)

	vm, err := wasm.NewVM()
	require.NoError(t, err)

	err = wasi.NewEnvironment().RegisterToVirtualMachine(vm)
	require.NoError(t, err)

	err = vm.InstantiateModule(mod, "test")
	require.NoError(t, err)

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := vm.ExecExportedFunction("test", "fibonacci", uint64(c.in))
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
