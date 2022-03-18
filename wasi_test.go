package wazero

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestStartWASICommand_UsesStoreContext(t *testing.T) {
	type key string
	config := NewRuntimeConfig().WithContext(context.WithValue(context.Background(), key("wa"), "zero"))
	r := NewRuntimeWithConfig(config)

	// Define a function that will be re-exported as the WASI function: _start
	var calledStart bool
	start := func(ctx wasm.Module) {
		calledStart = true
		require.Equal(t, config.ctx, ctx.Context())
	}

	_, err := r.NewModuleBuilder("").ExportFunction("start", start).Instantiate()
	require.NoError(t, err)

	_, err = r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)

	decoded, err := r.CompileModule([]byte(`(module $wasi_test.go
	(import "" "start" (func $start))
	(memory 1)
	(export "_start" (func $start))
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)

	// Start the module as a WASI command. This will fail if the context wasn't as intended.
	_, err = StartWASICommand(r, decoded)
	require.NoError(t, err)
	require.True(t, calledStart)
}

// wasiArg was compiled from examples/testdata/wasi_arg.wat
//go:embed examples/testdata/wasi_arg.wasm
var wasiArg []byte

func TestStartWASICommandWithConfig(t *testing.T) {
	r := NewRuntime()

	stdout := bytes.NewBuffer(nil)

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := NewSysConfig().WithStdout(stdout)
	wasi, err := r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	m, err := r.CompileModule(wasiArg)
	require.NoError(t, err)

	// Re-use the same module many times.
	for _, tc := range []string{"a", "b", "c"} {
		mod, err := StartWASICommandWithConfig(r, m.WithName(tc), sys.WithArgs(tc))
		require.NoError(t, err)

		// Ensure the scoped configuration applied. As the args are null-terminated, we append zero (NUL).
		require.Equal(t, append([]byte(tc), 0), stdout.Bytes())

		stdout.Reset()
		require.NoError(t, mod.Close())

		// TODO: figure out how to test config closed.
	}
}
