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
	allocateBuffer := func(context.Context, uint32) uint32 {
		panic("unimplemented")
	}

	// Host-side implementation of get_random_string on Wasm import.
	getRandomString := func(ctx wasm.ModuleContext, retBufPtr uint32, retBufSize uint32) {
		// Assert that context values passed in from CallFunctionContext are accessible.
		contextValue := ctx.Context().Value(testKey{}).(int64)
		require.Equal(t, int64(12345), contextValue)

		const bufferSize = 10000 // force memory space grow to ensure eager failures on missing setup
		// Allocate the in-Wasm memory region so we can store the generated string.
		// Note that this is recursive call. That means that this is the VM function call during the VM function call.
		// More precisely, we call test.base64 (in Wasm), and the function in turn calls this get_random_string function,
		// and we call test.allocate_buffer (in Wasm) here: host->vm->host->vm.
		offset := allocateBuffer(ctx.Context(), bufferSize)

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

	mod, err := wazero.DecodeModuleBinary(hostFuncWasm)
	require.NoError(t, err)

	store := wazero.NewStore()

	_, err = wazero.ExportHostFunctions(store, "env", map[string]interface{}{"get_random_string": getRandomString})
	require.NoError(t, err)

	// Note: host_func.go doesn't directly use WASI, but TinyGo needs to be initialized as a WASI Command.
	_, err = wazero.ExportHostFunctions(store, wasi.ModuleSnapshotPreview1, wazero.WASISnapshotPreview1())
	require.NoError(t, err)

	exports, err := wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	allocateBufferFn := exports.Function("allocate_buffer")

	// Implement the function pointer. This mainly shows how you can decouple a module function dependency.
	allocateBuffer = func(ctx context.Context, size uint32) uint32 {
		res, err := allocateBufferFn(ctx, uint64(size))
		require.NoError(t, err)
		return uint32(res[0])
	}

	// Set a context variable that should be available in wasm.ModuleContext.
	ctx := context.WithValue(context.Background(), testKey{}, int64(12345))

	// Invoke a module-defined function that depends on a host function import
	_, err = exports.Function("base64")(ctx, uint64(5))
	require.NoError(t, err)
}
