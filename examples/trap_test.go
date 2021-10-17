package examples

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func Test_trap(t *testing.T) {
	buf, err := os.ReadFile("wasm/trap.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	vm, err := wasm.NewVM()
	require.NoError(t, err)

	err = wasi.NewEnvironment().RegisterToVirtualMachine(vm)
	require.NoError(t, err)

	err = vm.InstantiateModule(mod, "test")
	require.NoError(t, err)

	_, _, err = vm.ExecExportedFunction("test", "cause_panic")
	require.Error(t, err)
	require.True(t, errors.Is(err, wasm.ErrFunctionTrapped))
}
