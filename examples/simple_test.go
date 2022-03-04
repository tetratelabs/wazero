package examples

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

// Test_Simple implements a basic function in go: hello. This is imported as the Wasm name "$hello" and run on start.
func Test_Simple(t *testing.T) {
	stdout := new(bytes.Buffer)
	goFunc := func() {
		_, _ = fmt.Fprintln(stdout, "hello!")
	}

	r := wazero.NewRuntime()

	// Host functions can be exported as any module name, including the empty string.
	env := &wazero.HostModuleConfig{Name: "", Functions: map[string]interface{}{"hello": goFunc}}
	_, err := r.NewHostModule(env)
	require.NoError(t, err)

	// The "hello" function was imported as $hello in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "hello" was called!
	_, err = r.NewModuleFromSource([]byte(`(module $test
	(import "" "hello" (func $hello))
	(start $hello)
)`))
	require.NoError(t, err)
	require.Equal(t, "hello!\n", stdout.String())
}
