package wasi

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/heeus/inv-wazero"
)

// wasiArg was compiled from testdata/wasi_arg.wat
//go:embed testdata/wasi_arg.wasm
var wasiArg []byte

func TestInstantiateModuleWithConfig(t *testing.T) {
	r := wazero.NewRuntime()

	stdout := bytes.NewBuffer(nil)

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := wazero.NewModuleConfig().WithStdout(stdout)
	wm, err := InstantiateSnapshotPreview1(r)
	require.NoError(t, err)
	defer wm.Close()

	code, err := r.CompileModule(wasiArg)
	require.NoError(t, err)

	// Re-use the same module many times.
	for _, tc := range []string{"a", "b", "c"} {
		mod, err := r.InstantiateModuleWithConfig(code, sys.WithArgs(tc).WithName(tc))
		require.NoError(t, err)

		// Ensure the scoped configuration applied. As the args are null-terminated, we append zero (NUL).
		require.Equal(t, append([]byte(tc), 0), stdout.Bytes())

		stdout.Reset()
		require.NoError(t, mod.Close())
	}
}
