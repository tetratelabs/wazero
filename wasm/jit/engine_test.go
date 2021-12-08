package jit

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func fibonacci(in uint64) uint64 {
	if in <= 1 {
		return 1
	}
	return fibonacci(in-1) + fibonacci(in-2)
}

func TestEngine_fibonacci(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip()
	}
	buf, err := os.ReadFile("testdata/fib.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(wazeroir.NewEngine())
	require.NoError(t, err)
	err = wasi.NewEnvironment().Register(store)
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	m, ok := store.ModuleInstances["test"]
	require.True(t, ok)
	exp, ok := m.Exports["fib"]
	require.True(t, ok)
	f := exp.Function
	eng := newEngine()
	err = eng.PreCompile([]*wasm.FunctionInstance{f})
	require.NoError(t, err)
	err = eng.Compile(f)
	require.NoError(t, err)
	for _, in := range []uint64{5, 10, 20} {
		out, err := eng.Call(f, in)
		require.NoError(t, err)
		require.Equal(t, fibonacci(in), out[0])
	}
}
