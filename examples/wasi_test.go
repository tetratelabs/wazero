package examples

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

func Test_WASI(t *testing.T) {
	// built-in WASI function to write a random value to memory
	randomGet := func(ctx wasm.ModuleContext, buf, bufLen uint32) wasi.Errno {
		panic("unimplemented")
	}

	stdout := new(bytes.Buffer)
	goFunc := func(ctx wasm.ModuleContext) {
		// Write 8 random bytes to memory using WASI.
		errno := randomGet(ctx, 0, 8)
		require.Equal(t, wasi.ErrnoSuccess, errno)

		// Read them back and print it in hex!
		random, ok := ctx.Memory().ReadUint64Le(0)
		require.True(t, ok)
		_, _ = fmt.Fprintf(stdout, "random: %x\n", random)
	}

	store := wazero.NewStore()

	// Host functions can be exported as any module name, including the empty string.
	env := &wazero.HostModuleConfig{Name: "", Functions: map[string]interface{}{"random": goFunc}}
	_, err := wazero.InstantiateHostModule(store, env)

	// Configure WASI and implement the function to use it
	we, err := wazero.InstantiateHostModule(store, wazero.WASISnapshotPreview1())
	require.NoError(t, err)
	randomGetFn := we.Function("random_get")

	// Implement the function pointer. This mainly shows how you can decouple a host function dependency.
	randomGet = func(ctx wasm.ModuleContext, buf, bufLen uint32) wasi.Errno {
		res, err := randomGetFn(ctx, uint64(buf), uint64(bufLen))
		require.NoError(t, err)
		return wasi.Errno(res[0])
	}

	// The "random" function was imported as $random in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "random" was called!
	_, err = wazero.InstantiateModule(store, &wazero.ModuleConfig{Source: []byte(`(module $wasi
	(import "wasi_snapshot_preview1" "random_get"
		(func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))
	(import "" "random" (func $random))
	(memory 1)
	(start $random)
)`)})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "random: ")
}
