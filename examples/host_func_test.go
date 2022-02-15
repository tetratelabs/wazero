package examples

import (
	"context"
	"crypto/rand"
	_ "embed"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	binaryFormat "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

type testKey struct{}

//go:embed testdata/host_func.wasm
var hostFuncWasm []byte

func Test_hostFunc(t *testing.T) {
	mod, err := binaryFormat.DecodeModule(hostFuncWasm)
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())

	// Host-side implementation of get_random_string on Wasm import.
	getRandomString := func(ctx api.HostFunctionCallContext, retBufPtr uint32, retBufSize uint32) {
		// Assert that context values passed in from CallFunctionContext are accessible.
		contextValue := ctx.Context().Value(testKey{}).(int64)
		require.Equal(t, int64(12345), contextValue)

		const bufferSize = 10000 // force memory space grow to ensure eager failures on missing setup
		// Allocate the in-Wasm memory region so we can store the generated string.
		// Note that this is recursive call. That means that this is the VM function call during the VM function call.
		// More precisely, we call test.base64 (in Wasm), and the function in turn calls this get_random_string function,
		// and we call test.allocate_buffer (in Wasm) here: host->vm->host->vm.
		ret, _, err := store.CallFunction(ctx.Context(), "test", "allocate_buffer", bufferSize)
		require.NoError(t, err)
		require.Len(t, ret, 1)
		bufAddr := ret[0]

		// Store the address info to the memory.
		require.True(t, ctx.Memory().WriteUint32Le(retBufPtr, uint32(bufAddr)))
		require.True(t, ctx.Memory().WriteUint32Le(retBufSize, uint32(bufferSize)))

		// Now store the random values in the region.
		b, ok := ctx.Memory().Read(uint32(bufAddr), bufferSize)
		require.True(t, ok)

		n, err := rand.Read(b)
		require.NoError(t, err)
		require.Equal(t, bufferSize, n)
	}

	err = store.AddHostFunction("env", "get_random_string", reflect.ValueOf(getRandomString))
	require.NoError(t, err)

	err = wasi.RegisterAPI(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	ctx := context.Background()

	// We assume that TinyGo binary expose "_start" symbol
	// to initialize the memory state.
	// Meaning that TinyGo binary is "WASI command":
	// https://github.com/WebAssembly/WASI/blob/main/design/application-abi.md
	_, _, err = store.CallFunction(ctx, "test", "_start")
	require.NoError(t, err)

	// Set a context variable that should be available in api.hostFunctionCallContext.
	ctx = context.WithValue(ctx, testKey{}, int64(12345))

	_, _, err = store.CallFunction(ctx, "test", "base64", 5)
	require.NoError(t, err)
}
