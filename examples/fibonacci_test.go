package examples

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func Test_fibonacci(t *testing.T) {
	buf, err := os.ReadFile("testdata/fibonacci.wasm")
	require.NoError(t, err)

	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)

	store := wasm.NewStore(wazeroir.NewEngine())
	require.NoError(t, err)

	err = wasi.NewEnvironment().Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	for _, c := range []struct {
		in, exp int32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, retTypes, err := store.CallFunction("test", "fibonacci", uint64(c.in))
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.exp, int32(ret[0]))
	}
}
