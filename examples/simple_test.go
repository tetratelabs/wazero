package examples

import (
	"bytes"
	"context"
	"fmt"
	"github.com/tetratelabs/wazero/wasm"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

// Test_Simple implements a basic function in go: hello. This is imported as the Wasm name "$hello" and run on start.
func Test_Simple(t *testing.T) {
	mod, err := wazero.DecodeModuleText([]byte(`(module $test
	(import "" "hello" (func $hello))
	(start $hello)
)`))
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
		addInts := func(ctx wasm.HostFunctionCallContext, offset uint32) uint32 {
			x, _ := ctx.Memory().ReadUint32Le(offset)
			, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
			// add a little extra if we put some in the context!
			return x + y + ctx.Value(extraKey).(uint32)
		}

	// Assign the Go function as a host function. This could error if the signature was invalid for Wasm.
	hostFuncs, err := wazero.NewHostFunctions(map[string]interface{}{"hello": goFunc})
	require.NoError(t, err)

	// Host functions can be exported as any module name, including the empty string.
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{"": hostFuncs},
	})
	require.NoError(t, err)

	// The "hello" function was imported as $hello in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "hello" was called!
	_, err = store.Instantiate(mod)
	require.NoError(t, err)
	require.Equal(t, "hello!\n", stdout.String())
}
