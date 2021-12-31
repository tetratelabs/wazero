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

// Test_Simple implements a basic function in go: hello. This is imported as the Wasm name "$hello" and run on start.
//	(module
//		(import "" "hello" (func $hello))
//		(start $hello)
//	)
func Test_Simple(t *testing.T) {
	buf, err := os.ReadFile("testdata/simple.wasm")
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

	// The "hello" function was imported as $hello in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "hello" was called!
	require.Equal(t, "hello!\n", stdout.String())
}
