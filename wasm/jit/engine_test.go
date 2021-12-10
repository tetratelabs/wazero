package jit

import (
	"os"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
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
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	out, _, err := store.CallFunction("test", "fib", 20)
	require.NoError(t, err)
	require.Equal(t, uint64(10946), out[0])
}

func TestEngine_unreachable(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip()
	}
	buf, err := os.ReadFile("testdata/unreachable.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	_, _, err = store.CallFunction("test", "cause_unreachable")
	exp := `wasm runtime error: unreachable
wasm backtrace:
	0: three
	1: two
	2: one
	3: cause_unreachable`
	require.Error(t, err)
	require.Equal(t, exp, err.Error())
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
	require.Len(t, eng.compiledHostFunctions, 1)
	require.Len(t, eng.compiledHostFunctionIndex, 1)
	prevCompiledFunctions := make([]*compiledWasmFunction, len(eng.compiledWasmFunctions))
	prevHostFunctions := make([]*compiledHostFunction, len(eng.compiledHostFunctions))
	copy(prevCompiledFunctions, eng.compiledWasmFunctions)
	copy(prevHostFunctions, eng.compiledHostFunctions)
	err = eng.PreCompile(fs)
	// Precompiling same functions should be noop.
	require.NoError(t, err)
	require.Len(t, eng.compiledWasmFunctionIndex, 3)
	require.Len(t, eng.compiledHostFunctionIndex, 1)
	require.Equal(t, prevHostFunctions, eng.compiledHostFunctions)
	require.Equal(t, prevCompiledFunctions, eng.compiledWasmFunctions)
}

func TestEngine_maybeGrowStack(t *testing.T) {
	t.Run("grow", func(t *testing.T) {
		eng := &engine{stack: make([]uint64, 10)}
		eng.currentBaseStackPointer = 5
		eng.push(10)
		require.Equal(t, uint64(1), eng.currentStackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
		eng.maybeGrowStack(100)
		// Currently we have 9 empty slots (10 - 1(base pointer)) above base pointer for new items,
		// but we require 100 max stack pointer for the next function,
		// so this results in making the stack length 120 = 10(current len)*2+(100(maxStackPointer))
		require.Len(t, eng.stack, 120)
		// maybeAdjustStack only shrink the stack,
		// and must not modify neither stack pointer nor the values in the stack.
		require.Equal(t, uint64(1), eng.currentStackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
	})
	t.Run("noop", func(t *testing.T) {
		eng := &engine{stack: make([]uint64, 10)}
		eng.currentBaseStackPointer = 1
		eng.push(10)
		require.Equal(t, uint64(1), eng.currentStackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
		eng.maybeGrowStack(6)
		// Currently we have 9 empty slots (10 - 1(base pointer)) above base pointer for new items,
		// and we only require 6 max stack pointer for the next function, so we have enough empty slots.
		// so maybeGrowStack must not modify neither stack pointer, the values in the stack nor stack len.
		require.Len(t, eng.stack, 10)
		require.Equal(t, uint64(1), eng.currentStackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.currentBaseStackPointer+eng.currentStackPointer-1])
	})
}
