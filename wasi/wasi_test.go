package wasi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

func TestArgs(t *testing.T) {
	buf, err := os.ReadFile("testdata/args.wasm")
	require.NoError(t, err)

	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())
	require.NoError(t, err)

	args, err := Args([]string{"foo", "bar", "foobar", "", "baz"})
	require.NoError(t, err)
	wasiEnv := NewEnvironment(args)

	err = wasiEnv.Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "_start")
	require.NoError(t, err)
}
