package wasi_snapshot_preview1

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// wasiArg was compiled from testdata/wasi_arg.wat
//
//go:embed testdata/wasi_arg.wasm
var wasiArg []byte

func TestInstantiateModule(t *testing.T) {
	r := wazero.NewRuntime()

	stdout := bytes.NewBuffer(nil)

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := wazero.NewModuleConfig().WithStdout(stdout)
	wm, err := Instantiate(testCtx, r)
	require.NoError(t, err)
	defer wm.Close(testCtx)

	compiled, err := r.CompileModule(testCtx, wasiArg, wazero.NewCompileConfig())
	require.NoError(t, err)
	defer compiled.Close(testCtx)

	// Re-use the same module many times.
	tests := []string{"a", "b", "c"}

	for _, tt := range tests {
		tc := tt
		mod, err := r.InstantiateModule(testCtx, compiled, sys.WithArgs(tc).WithName(tc))
		require.NoError(t, err)

		// Ensure the scoped configuration applied. As the args are null-terminated, we append zero (NUL).
		require.Equal(t, append([]byte(tc), 0), stdout.Bytes())

		stdout.Reset()
		require.NoError(t, mod.Close(testCtx))
	}
}
