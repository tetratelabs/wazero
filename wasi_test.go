package wazero

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

func TestStartWASICommand_UsesStoreContext(t *testing.T) {
	type key string
	config := &StoreConfig{Context: context.WithValue(context.Background(), key("wa"), "zero")}
	store := NewStoreWithConfig(config)

	// Define a function that will be re-exported as the WASI function: _start
	var calledStart bool
	start := func(ctx wasm.ModuleContext) {
		calledStart = true
		require.Equal(t, config.Context, ctx.Context())
	}
	_, err := ExportHostFunctions(store, "", map[string]interface{}{"start": start})
	require.NoError(t, err)

	mod, err := DecodeModuleText([]byte(`(module $wasi_test.go
	(import "" "start" (func $start))
	(memory 1)
	(export "_start" (func $start))
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)

	_, err = ExportHostFunctions(store, wasi.ModuleSnapshotPreview1, WASISnapshotPreview1())
	require.NoError(t, err)

	// Start the module as a WASI command. This will fail if the context wasn't as intended.
	_, err = StartWASICommand(store, mod)
	require.NoError(t, err)
	require.True(t, calledStart)
}
