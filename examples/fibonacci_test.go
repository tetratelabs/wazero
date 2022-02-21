package examples

import (
	"context"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
)

// fibWasm was compiled from TinyGo testdata/fibonacci.go
//go:embed testdata/fibonacci.wasm
var fibWasm []byte // TODO: implement this in text format as it is less distracting setup

func Test_fibonacci(t *testing.T) {
	mod, err := wazero.DecodeModuleBinary(fibWasm)
	require.NoError(t, err)

	store := wazero.NewStore()

	// Note: fibonacci.go doesn't directly use WASI, but TinyGo needs to be initialized as a WASI Command.
	_, err = wazero.ExportHostFunctions(store, wasi.ModuleSnapshotPreview1, wazero.WASISnapshotPreview1())
	require.NoError(t, err)

	exports, err := wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	fibonacci, ok := exports.Function("fibonacci")
	require.True(t, ok)

	for _, c := range []struct {
		input, expected uint64 // i32_i32 sig, but wasm.Function params and results are uint64
	}{
		{input: 20, expected: 6765},
		{input: 10, expected: 55},
		{input: 5, expected: 5},
	} {
		results, err := fibonacci(context.Background(), c.input)
		require.NoError(t, err)
		require.Equal(t, c.expected, results[0])
	}
}
