package examples

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func Test_hostFunc(t *testing.T) {
	buf, err := os.ReadFile("wasm/host_func.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	store := wasm.NewStore(wazeroir.NewEngine())

	// Host-side implementation of get_random_string on Wasm import.
	getRandomString := func(ctx *wasm.HostFunctionCallContext, retBufPtr uint32, retBufSize uint32) {
		const bufferSize = 10
		// Allocate the in-Wasm memory region so we can store the generated string.
		// Note that this is recursive call. That means that this is the VM function call during the VM function call.
		// More precisely, we call test.base64 (in Wasm), and the function in turn calls this get_random_string function,
		// and we call test.allocate_buffer (in Wasm) here: host->vm->host->vm.
		ret, _, err := store.CallFunction("test", "allocate_buffer", bufferSize)
		require.NoError(t, err)
		require.Len(t, ret, 1)
		bufAddr := ret[0]

		// Store the address info to the memory.
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufPtr:], uint32(bufAddr))
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufSize:], bufferSize)

		// Now store the random values in the region.
		n, err := rand.Read(ctx.Memory.Buffer[bufAddr : bufAddr+bufferSize])
		require.NoError(t, err)
		require.Equal(t, bufferSize, n)
	}

	err = store.AddHostFunction("env", "get_random_string", reflect.ValueOf(getRandomString))
	require.NoError(t, err)

	err = wasi.NewEnvironment().Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "base64", 5)
	require.NoError(t, err)
}
