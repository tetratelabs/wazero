package examples

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func Test_trap(t *testing.T) {
	buf, err := os.ReadFile("testdata/trap.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	store := wasm.NewStore(wazeroir.NewEngine())

	err = wasi.NewEnvironment().Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "cause_panic")
	require.Error(t, err)

	const expErrMsg = `wasm runtime error: unreachable
wasm backtrace:
	0: runtime._panic
	1: main.three
	2: main.two
	3: main.one
	4: cause_panic`
	require.Equal(t, expErrMsg, err.Error())
}
