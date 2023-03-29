package wasi_snapshot_preview1_test

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// pringArgsWasm was compiled from testdata/wasi_arg.wat
//
//go:embed testdata/print_args.wasm
var pringArgsWasm []byte

func TestInstantiateModule(t *testing.T) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var stdout bytes.Buffer

	// Configure WASI to write stdout to a buffer, so that we can verify it later.
	sys := wazero.NewModuleConfig().WithStdout(&stdout)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, pringArgsWasm)
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

// printPrestatDirname was compiled from testdata/print_prestat_dirname.wat
//
//go:embed testdata/print_prestat_dirname.wasm
var printPrestatDirname []byte

// TestInstantiateModule_Prestat
func TestInstantiateModule_Prestat(t *testing.T) {
	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	var stdout bytes.Buffer

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	_, err := r.InstantiateWithConfig(ctx, printPrestatDirname, wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithFSConfig(wazero.NewFSConfig().WithFSMount(fstest.FS, "/wazero")))
	require.NoError(t, err)

	require.Equal(t, "/wazero", stdout.String())
}
