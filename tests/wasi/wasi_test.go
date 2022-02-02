// package wasi tests wazero's WASI wasi_snapshot_preview1 is compatible with at least one language runtime.
// Links:
//    - Spec: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
//    - Witx: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/witx/wasi_snapshot_preview1.witx
//
// We use tinygo, to perform these tests as the maintainers are familiar with it.
//
// Note: Use `make build.tests-wasi` to compile `testdata/*.wasm` from `testdata/*.go`
// Note: This also substitutes for WASI spec tests until we have another option
package wasi

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
// size of the result of args_get, so TinyGo calls the both of them together to retrieve the
// WASI's arguments. Any other language runtime would do the same things.
func Test_ArgsSizesGet_ArgsGet(t *testing.T) {
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

			// Calling `_start` to call WASI APIs because it's the only stable way to call WASI APIs on TinyGo.
			_, _, err = store.CallFunction("test", "_start")
			require.NoError(t, err)

			require.Equal(t, tc.expectedArgs, strings.TrimSpace(stdoutBuf.String()))
		})
	}
}
