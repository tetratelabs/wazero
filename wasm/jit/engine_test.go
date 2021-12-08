package jit

import (
	"os"
	"reflect"
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

func TestEngine_PreCompile(t *testing.T) {
	eng := newEngine()
	hf := reflect.ValueOf(func(*wasm.HostFunctionCallContext) {})
	fs := []*wasm.FunctionInstance{
		{HostFunction: &hf},
		{HostFunction: nil},
		{HostFunction: nil},
		{HostFunction: nil},
	}
	err := eng.PreCompile(fs)
	require.NoError(t, err)
	require.Len(t, eng.compiledWasmFunctions, 3)
	require.Len(t, eng.compiledWasmFunctionIndex, 3)
	require.Len(t, eng.hostFunctions, 1)
	require.Len(t, eng.hostFunctionIndex, 1)
	err = eng.PreCompile(fs)
	// Precompiling same functions should be noop.
	require.NoError(t, err)
	require.Len(t, eng.compiledWasmFunctions, 3)
	require.Len(t, eng.compiledWasmFunctionIndex, 3)
	require.Len(t, eng.hostFunctions, 1)
	require.Len(t, eng.hostFunctionIndex, 1)
	// Check the indexes.
	require.Equal(t, int64(0), eng.hostFunctionIndex[fs[0]])
	require.Equal(t, int64(0), eng.compiledWasmFunctionIndex[fs[1]])
	require.Equal(t, int64(1), eng.compiledWasmFunctionIndex[fs[2]])
	require.Equal(t, int64(2), eng.compiledWasmFunctionIndex[fs[3]])
}

func TestEngine_stackGrow(t *testing.T) {
	eng := newEngine()
	require.Len(t, eng.stack, initialStackSize)
	eng.push(10)
	require.Equal(t, uint64(1), eng.currentStackPointer)
	eng.stackGrow()
	require.Len(t, eng.stack, initialStackSize*2)
	require.Equal(t, uint64(1), eng.currentStackPointer)
	require.Equal(t, uint64(10), eng.stack[eng.currentStackPointer-1])
}
