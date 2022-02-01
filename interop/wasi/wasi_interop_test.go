// package wasi_interop tests wazero's WASI wasi_snapshot_preview1 is compatible with at least one runtime.
//
// We use tinygo, to perform these tests as the maintainers are familiar with it.
//
// Note: Use `make build.interop` to compile `testdata/*.wasm` from `testdata/*.go`
// Note: This also substitutes for WASI spec tests until we have another option
package wasi_interop

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	wbinary "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

// Test args_sizes_get and args_get. args_sizes_get must be used to know the length and the
// size of the result of args_get, so both of them are used at the same time to retreive the
// WASI's arguments.
func Test_ArgsAPI(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedArgs string
	}{
		{
			name:         "empty",
			args:         []string{},
			expectedArgs: "os.Args: []",
		},
		{
			name:         "simple",
			args:         []string{"foo", "bar", "foobar", "", "baz"},
			expectedArgs: "os.Args: [foo bar foobar  baz]",
		},
	}

	buf, err := os.ReadFile("testdata/args.wasm")
	require.NoError(t, err)

	mod, err := wbinary.DecodeModule(buf)
	require.NoError(t, err)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			store := wasm.NewStore(interpreter.NewEngine())
			require.NoError(t, err)

			stdoutBuf := bytes.NewBuffer(nil)
			args, err := wasi.Args(tc.args)
			require.NoError(t, err)
			wasiEnv := wasi.NewEnvironment(args, wasi.Stdout(stdoutBuf))

			err = wasiEnv.Register(store)
			require.NoError(t, err)

			err = store.Instantiate(mod, "test")
			require.NoError(t, err)

			// XXX Strictly speaking, this test code violates the WASI specification.
			// The WASI specification does not guarantee that a function exported from a WASI command
			// can be called outside the context of `_start`.
			//   > Command instances may assume that none of their exports are accessed outside the duraction of that call.
			// Link: https://github.com/WebAssembly/WASI/blob/db4e3a12dadbe3e7e41dddd04888db3bf1cf7a96/design/application-abi.md
			// In fact, calling a WASI function from a normal exported function without calling `_start` first in TinyGo crashes.
			//
			// However, once `_start` is called, it appears that WASI functions can be called from exported functions.
			// We believe it's unlikely TinyGo wil break this behavior in the future.
			// So, we call the test helper functions directly after calling `_start` once for more concise testing.

			// Let TinyGo runtime initialize the WASI environment by calling _start
			_, _, err = store.CallFunction("test", "_start")
			require.NoError(t, err)

			// Call a test function directly
			_, _, err = store.CallFunction("test", "PrintArgs")
			require.NoError(t, err)

			require.Equal(t, tc.expectedArgs, strings.TrimSpace(stdoutBuf.String()))
		})
	}
}
