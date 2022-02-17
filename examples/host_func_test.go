package examples

import (
	"context"
	"crypto/rand"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

type testKey struct{}

// hostFuncWasm was compiled from TinyGo testdata/host_func.go
//go:embed testdata/host_func.wasm
var hostFuncWasm []byte

func Test_hostFunc(t *testing.T) {
	mod, err := wazero.DecodeModuleBinary(hostFuncWasm)
	require.NoError(t, err)

	// Host-side implementation of get_random_string on Wasm import.
	getRandomString := func(ctx wasm.HostFunctionCallContext, retBufPtr uint32, retBufSize uint32) {
		// Assert that context values passed in from CallFunctionContext are accessible.
		contextValue := ctx.Context().Value(testKey{}).(int64)
		require.Equal(t, int64(12345), contextValue)

		const bufferSize = 10000 // force memory space grow to ensure eager failures on missing setup
		// Allocate the in-Wasm memory region so we can store the generated string.
		// Note that this is recursive call. That means that this is the VM function call during the VM function call.
		// More precisely, we call test.base64 (in Wasm), and the function in turn calls this get_random_string function,
		// and we call test.allocate_buffer (in Wasm) here: host->vm->host->vm.
		allocateBuffer, ok := ctx.Functions().GetFunctionI32Return("allocate_buffer")
		require.True(t, ok)

		offset, err := allocateBuffer(ctx.Context(), bufferSize)
		require.NoError(t, err)

		// Store the address info to the memory.
		require.True(t, ctx.Memory().WriteUint32Le(retBufPtr, offset))
		require.True(t, ctx.Memory().WriteUint32Le(retBufSize, uint32(bufferSize)))

		// Now store the random values in the region.
		b, ok := ctx.Memory().Read(offset, bufferSize)
		require.True(t, ok)

		n, err := rand.Read(b)
		require.NoError(t, err)
		require.Equal(t, bufferSize, n)
	}

	hfs, err := wazero.NewHostFunctions(map[string]interface{}{"get_random_string": getRandomString})
	require.NoError(t, err)

	// Note: neither the host function above nor the TinyGo source testdata/host_func.go directly use WASI.
	// However, all TinyGo binaries must be treated as WASI Commands to initialize memory.
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{
			wasi.ModuleSnapshotPreview1: wazero.WASISnapshotPreview1(),
			"env":                       hfs,
		},
	})
	require.NoError(t, err)

	m, err := wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	// Set a context variable that should be available in wasm.HostFunctionCallContext.
	ctx := context.WithValue(context.Background(), testKey{}, int64(12345))
	base64, ok := m.GetFunctionVoidReturn("base64")
	require.True(t, ok)
	err = base64(ctx, 5)
	require.NoError(t, err)
}
