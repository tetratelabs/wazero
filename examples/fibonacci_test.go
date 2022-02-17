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

	// Note: the TinyGo function fibonacci doesn't directly use WASI.
	// However, all TinyGo binaries must be treated as WASI Commands to initialize memory.
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{
			wasi.ModuleSnapshotPreview1: wazero.WASISnapshotPreview1(),
		},
	})
	require.NoError(t, err)

	m, err := wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	fibonacci, ok := m.GetFunctionI32Return("fibonacci")
	require.True(t, ok)

	for _, c := range []struct {
		in, exp uint32
	}{
		{in: 20, exp: 6765},
		{in: 10, exp: 55},
		{in: 5, exp: 5},
	} {
		ret, err := fibonacci(context.Background(), uint64(c.in)) // Params are uint64, but evaluated per the signature.
		require.NoError(t, err)
		require.Equal(t, c.exp, ret)
	}
}
