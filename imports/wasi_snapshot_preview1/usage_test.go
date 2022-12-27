package wasi_snapshot_preview1_test

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// wasiArg was compiled from testdata/wasi_arg.wat
//
//go:embed testdata/wasi_arg.wasm
var wasiArg []byte

func TestInstantiateModule(t *testing.T) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var stdout bytes.Buffer

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := wazero.NewModuleConfig().WithStdout(&stdout)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, wasiArg, wasm.CompileModuleOptions{})
	require.NoError(t, err)

	// Re-use the same module many times.
	tests := []string{"a", "b", "c"}

	for _, tt := range tests {
		tc := tt
		mod, err := r.InstantiateModule(ctx, compiled, sys.WithArgs(tc).WithName(tc))
		require.NoError(t, err)

		// Ensure the scoped configuration applied. As the args are null-terminated, we append zero (NUL).
		require.Equal(t, append([]byte(tc), 0), stdout.Bytes())

		stdout.Reset()
		require.NoError(t, mod.Close(ctx))
	}
}
