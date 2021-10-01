package spectest

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func requireInitVM(t *testing.T, name string, modules map[string]*wasm.Module) *wasm.VirtualMachine {
	buf, err := os.ReadFile(fmt.Sprintf("./cases/%s.wasm", name))
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	vm, err := wasm.NewVM(mod, modules)
	require.NoError(t, err)
	return vm
}
