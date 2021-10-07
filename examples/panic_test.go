package examples

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func Test_panic(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/panic.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	vm, err := wasm.NewVM()
	require.NoError(t, err)

	err = wasi.NewEnvironment().RegisterToVirtualMachine(vm)
	require.NoError(t, err)

	err = vm.InstantiateModule(mod, "test")
	require.NoError(t, err)

	defer func() {
		err := recover()
		require.Equal(t, "unreachable", err)
	}()
	vm.ExecExportedFunction("test", "cause_panic")
}
