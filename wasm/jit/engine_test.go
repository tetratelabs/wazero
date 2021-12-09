package jit

import (
	"os"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

// Ensures that the offset consts do not drift when we manipulate the engine struct.
func TestEngine_veifyOffsetValue(t *testing.T) {
	require.Equal(t, int(unsafe.Offsetof((&engine{}).stack)), engineStackSliceOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).currentStackPointer)), engineCurrentStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).currentBaseStackPointer)), engineCurrentBaseStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).jitCallStatusCode)), engineJITCallStatusCodeOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).functionCallIndex)), engineFunctionCallIndexOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).continuationAddressOffset)), engineContinuationAddressOffset)
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
	out, err := eng.Call(f, 20)
	require.NoError(t, err)
	require.Equal(t, uint64(10946), out[0])
}

func TestEngine_PreCompile(t *testing.T) {
	eng := newEngine()
	hf := reflect.ValueOf(func(*wasm.HostFunctionCallContext) {})
	// Usually the function instances in a module consist of the maxture
	// of host functions and wasm functions. And we treat a fcuntion instance
	// as a native one when .HostFunction is nil.
	fs := []*wasm.FunctionInstance{
		{HostFunction: &hf},
		{HostFunction: nil},
		{HostFunction: nil},
		{HostFunction: nil},
	}
	err := eng.PreCompile(fs)
	require.NoError(t, err)
	// Check the indexes.
	require.Len(t, eng.compiledWasmFunctions, 3)
	require.Len(t, eng.compiledWasmFunctionIndex, 3)
	require.Len(t, eng.hostFunctions, 1)
	require.Len(t, eng.hostFunctionIndex, 1)
	prevCompiledFunctions := make([]*compiledWasmFunction, len(eng.compiledWasmFunctions))
	prevHostFunctions := make([]hostFunction, len(eng.hostFunctions))
	copy(prevCompiledFunctions, eng.compiledWasmFunctions)
	copy(prevHostFunctions, eng.hostFunctions)
	err = eng.PreCompile(fs)
	// Precompiling same functions should be noop.
	require.NoError(t, err)
	require.Len(t, eng.compiledWasmFunctionIndex, 3)
	require.Len(t, eng.hostFunctionIndex, 1)
	require.Equal(t, prevHostFunctions, eng.hostFunctions)
	require.Equal(t, prevCompiledFunctions, eng.compiledWasmFunctions)
}

func TestEngine_stackGrow(t *testing.T) {
	eng := newEngine()
	require.Len(t, eng.stack, initialStackSize)
	eng.push(10)
	require.Equal(t, uint64(1), eng.currentStackPointer)
	require.Equal(t, uint64(10), eng.stack[eng.currentStackPointer-1])
	eng.stackGrow()
	require.Len(t, eng.stack, initialStackSize*2)
	// stackGrow only grows the stack len,
	// and must not modify neither stack pointer nor the values in the stack.
	require.Equal(t, uint64(1), eng.currentStackPointer)
	require.Equal(t, uint64(10), eng.stack[eng.currentStackPointer-1])
}
