package examples

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

func Test_WASI_args(t *testing.T) {
	buf, err := os.ReadFile("testdata/wasi_args.wasm")
	require.NoError(t, err)

	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())
	require.NoError(t, err)

	args, err := wasi.Args([]string{"foo", "bar", "foobar", "", "baz"})
	require.NoError(t, err)
	wasiEnv := wasi.NewEnvironment(args, wasi.Stdout(os.Stderr))

	err = wasiEnv.Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	// Let TinyGo runtime initialize the WASI environment by calling main.
	_, _, err = store.CallFunction("test", "_start")
	require.NoError(t, err)
}
