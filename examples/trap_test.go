package examples

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func Test_trap(t *testing.T) {
	buf, err := os.ReadFile("wasm/trap.wasm")
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
	fmt.Println(err)
}
