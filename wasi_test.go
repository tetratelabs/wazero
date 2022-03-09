package wazero

import (
	"context"
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

	_, err := r.NewModuleBuilder("").ExportFunction("start", start).InstantiateModule()
	require.NoError(t, err)

	_, err = r.InstantiateModule(WASISnapshotPreview1())
	require.NoError(t, err)

	decoded, err := r.DecodeModule([]byte(`(module $wasi_test.go
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
