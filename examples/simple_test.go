package examples

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/text"
)

// Test_Simple implements a basic function in go: hello. This is imported as the Wasm name "$hello" and run on start.
func Test_Simple(t *testing.T) {
	mod, err := text.DecodeModule([]byte(`(module
	(import "" "hello" (func $hello))
	(start $hello)
)`))
	require.NoError(t, err)

	// Create a new store and add the function "hello" which the module imports
	store := wasm.NewStore(interpreter.NewEngine())

	stdout := new(bytes.Buffer)
	hostFunction := func(_ api.HostFunctionCallContext) {
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
