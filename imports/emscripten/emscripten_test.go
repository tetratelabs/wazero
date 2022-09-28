package emscripten

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	. "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// growWasm was compiled from testdata/grow.cc
//
//go:embed testdata/grow.wasm
var growWasm []byte

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// TestGrow is an integration test until we have an Emscripten example.
func TestGrow(t *testing.T) {
	var log bytes.Buffer

	// Set context to one that has an experimental listener
	ctx := context.WithValue(testCtx, FunctionListenerFactoryKey{}, logging.NewLoggingListenerFactory(&log))

	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	_, err := Instantiate(ctx, r)
	require.NoError(t, err)

	// Emscripten exits main with zero by default
	_, err = r.InstantiateModuleFromBinary(ctx, growWasm)
	require.Error(t, err)
	require.Zero(t, err.(*sys.ExitError).ExitCode())

	// We expect the memory no-op memory growth hook to be invoked as wasm.
	require.Contains(t, log.String(), "--> env.emscripten_notify_memory_growth(memory_index=0)")
}
