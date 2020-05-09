package examples

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/stretchr/testify/require"
)

func Test_fibonacci(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/fibonacci.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	vm, err := wasm.NewVM(mod, wasi.Modules)
	require.NoError(t, err)

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := vm.ExecExportedFunction("fib", uint64(c.in))
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
