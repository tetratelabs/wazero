package jit

import (
	"errors"
	"os"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

// Ensures that the offset consts do not drift when we manipulate the engine struct.
func TestEngine_veifyOffsetValue(t *testing.T) {
	require.Equal(t, int(unsafe.Offsetof((&engine{}).stack)), engineStackSliceOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).stackPointer)), enginestackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).stackBasePointer)), enginestackBasePointerOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).jitCallStatusCode)), engineJITCallStatusCodeOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).functionCallAddress)), engineFunctionCallAddressOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).continuationAddressOffset)), engineContinuationAddressOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).globalSliceAddress)), engineglobalSliceAddressOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).memorySliceLen)), engineMemorySliceLenOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).tableSliceAddress)), engineTableSliceAddressOffset)
	require.Equal(t, int(unsafe.Offsetof((&engine{}).tableSliceLen)), engineTableSliceLenOffset)
}

func TestEngine_fibonacci(t *testing.T) {
	buf, err := os.ReadFile("testdata/fib.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)

	// We execute 1000 times in order to ensure the JIT engine is stable under high concurrency
	// and we have no conflict with Go's runtime.
	const goroutines = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			store := wasm.NewStore(NewEngine())
			require.NoError(t, err)
			err = store.Instantiate(mod, "test")
			require.NoError(t, err)
			out, _, err := store.CallFunction("test", "fib", 20)
			require.NoError(t, err)
			require.Equal(t, uint64(10946), out[0])
		}()
	}
	wg.Wait()
}

func TestEngine_fac(t *testing.T) {
	buf, err := os.ReadFile("testdata/fac.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	for _, name := range []string{
		"fac-rec",
		"fac-iter",
		"fac-rec-named",
		"fac-iter-named",
		"fac-opt",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			out, _, err := store.CallFunction("test", name, 25)
			require.NoError(t, err)
			require.Equal(t, uint64(7034535277573963776), out[0])
		})
	}

	_, _, err = store.CallFunction("test", "fac-rec", 1073741824)
	require.True(t, errors.Is(err, wasm.ErrCallStackOverflow))
}

func TestEngine_unreachable(t *testing.T) {
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

func TestEngine_call_indirect(t *testing.T) {
	buf, err := os.ReadFile("testdata/call_indirect.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	for i := uint64(0); i < 5; i++ {
		out, _, err := store.CallFunction("test", "callt", i)
		require.NoError(t, err)
		require.Equal(t, i+1, out[0])
	}
}

func TestEngine_memory(t *testing.T) {
	buf, err := os.ReadFile("testdata/memory.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	// First, we have zero-length memory instance.
	out, _, err := store.CallFunction("test", "size")
	require.NoError(t, err)
	require.Equal(t, uint64(0), out[0])
	// Then grow the memory.
	const newPages uint64 = 10
	out, _, err = store.CallFunction("test", "grow", newPages)
	require.NoError(t, err)
	// Grow returns the previous number of memory pages, namely zero.
	require.Equal(t, uint64(0), out[0])
	// Now size should return the new pages -- 10.
	out, _, err = store.CallFunction("test", "size")
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
	// Growing memory with zero pages is valid but should be noop.
	out, _, err = store.CallFunction("test", "grow", 0)
	require.NoError(t, err)
	require.Equal(t, newPages, out[0])
}

func TestEngine_maybeGrowStack(t *testing.T) {
	t.Run("grow", func(t *testing.T) {
		eng := &engine{stack: make([]uint64, 10)}
		eng.stackBasePointer = 5
		eng.push(10)
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.stackBasePointer+eng.stackPointer-1])
		eng.maybeGrowStack(100)
		// Currently we have 9 empty slots (10 - 1(base pointer)) above base pointer for new items,
		// but we require 100 max stack pointer for the next function,
		// so this results in making the stack length 120 = 10(current len)*2+(100(maxStackPointer))
		require.Len(t, eng.stack, 120)
		// maybeAdjustStack only shrink the stack,
		// and must not modify neither stack pointer nor the values in the stack.
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.stackBasePointer+eng.stackPointer-1])
	})
	t.Run("noop", func(t *testing.T) {
		eng := &engine{stack: make([]uint64, 10)}
		eng.stackBasePointer = 1
		eng.push(10)
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.stackBasePointer+eng.stackPointer-1])
		eng.maybeGrowStack(6)
		// Currently we have 9 empty slots (10 - 1(base pointer)) above base pointer for new items,
		// and we only require 6 max stack pointer for the next function, so we have enough empty slots.
		// so maybeGrowStack must not modify neither stack pointer, the values in the stack nor stack len.
		require.Len(t, eng.stack, 10)
		require.Equal(t, uint64(1), eng.stackPointer)
		require.Equal(t, uint64(10), eng.stack[eng.stackBasePointer+eng.stackPointer-1])
	})
}
