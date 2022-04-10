package examples

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/heeus/inv-wazero"
)

// Test_Simple implements a basic function in go: hello. This is imported as the Wasm name "$hello" and run on start.
func Test_Simple(t *testing.T) {
	stdout := new(bytes.Buffer)
	hello := func() {
		_, _ = fmt.Fprintln(stdout, "hello!")
	}

	r := wazero.NewRuntime()

	// Host functions can be exported as any module name, including the empty string.
	host, err := r.NewModuleBuilder("").ExportFunction("hello", hello).Instantiate()
	require.NoError(t, err)
	defer host.Close()

	// The "hello" function was imported as $hello in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "hello" was called!
	mod, err := r.InstantiateModuleFromCode([]byte(`(module $test
	(import "" "hello" (func $hello))
	(start $hello)
)`))
	require.NoError(t, err)
	defer mod.Close()

	require.Equal(t, "hello!\n", stdout.String())
}
