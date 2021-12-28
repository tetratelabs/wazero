package examples

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

// Test_Simple implements a basic function in go ("hello"), called by a function defined Web Assembly ("run"):
func Test_Simple(t *testing.T) {
	// Decode simple.wasm which was pre-compiled from this text format:
	//	(module
	//		(import "" "hello" (func $hello))
	//		(func (export "run") (call $hello))
	//	)
	buf, err := os.ReadFile("wasm/simple.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)

	// Create a new store and add the function "hello" which the module imports
	store := wasm.NewStore(wazeroir.NewEngine())

	stdout := new(bytes.Buffer)
	hostFunction := func(_ *wasm.HostFunctionCallContext) {
		_, _ = fmt.Fprintln(stdout, "hello!")
	}

	require.NoError(t, store.AddHostFunction("", "hello", reflect.ValueOf(hostFunction)))

	// Now that the store has the prerequisite host function, instantiate the module.
	moduleName := "simple"
	require.NoError(t, store.Instantiate(mod, moduleName))

	// Finally, invoke the function "run" which calls "hello".
	// "run" has no parameters or result, so we can safely ignore all return values except the error.
	_, _, err = store.CallFunction(moduleName, "run")
	require.NoError(t, err)

	// Ensure the host function "hello" was actually called!
	require.Equal(t, "hello!\n", stdout.String())
}
