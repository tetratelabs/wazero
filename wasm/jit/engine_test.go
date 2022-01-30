package jit

import (
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/text"
)

// Ensures that the offset consts do not drift when we manipulate the target structs.
func TestVerifyOffsetValue(t *testing.T) {
	var eng engine
	// Offsets for engine.globalContext.
	require.Equal(t, int(unsafe.Offsetof(eng.valueStackElement0Address)), engineGlobalContextValueStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.valueStackLen)), engineGlobalContextValueStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.callFrameStackElementZeroAddress)), engineGlobalContextCallFrameStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.callFrameStackLen)), engineGlobalContextCallFrameStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.callFrameStackPointer)), engineGlobalContextCallFrameStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.previousCallFrameStackPointer)), engineGlobalContextPreviouscallFrameStackPointer)
	require.Equal(t, int(unsafe.Offsetof(eng.compiledFunctionsElement0Address)), engineGlobalContextCompiledFunctionsElement0AddressOffset)

	// Offsets for engine.moduleContext.
	require.Equal(t, int(unsafe.Offsetof(eng.moduleInstanceAddress)), engineModuleContextModuleInstanceAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.globalElement0Address)), engineModuleContextGlobalElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.memoryElement0Address)), engineModuleContextMemoryElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.memorySliceLen)), engineModuleContextMemorySliceLenOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.tableElement0Address)), engineModuleContextTableElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.tableSliceLen)), engineModuleContextTableSliceLenOffset)

	// Offsets for engine.valueStackContext
	require.Equal(t, int(unsafe.Offsetof(eng.stackPointer)), engineValueStackContextStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.stackBasePointer)), engineValueStackContextStackBasePointerOffset)

	// Offsets for engine.exitContext.
	require.Equal(t, int(unsafe.Offsetof(eng.statusCode)), engineExitContextJITCallStatusCodeOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.functionCallAddress)), engineExitContextFunctionCallAddressOffset)

	// Size and offsets for callFrame.
	var frame callFrame
	require.Equal(t, int(unsafe.Sizeof(frame)), callFrameDataSize)
	// Sizeof callframe must be a power of 2 as we do SHL on the index by "callFrameDataSizeMostSignificantSetBit" to obtain the offset address.
	require.True(t, callFrameDataSize&(callFrameDataSize-1) == 0)
	require.Equal(t, math.Ilogb(float64(callFrameDataSize)), callFrameDataSizeMostSignificantSetBit)
	require.Equal(t, int(unsafe.Offsetof(frame.returnAddress)), callFrameReturnAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(frame.returnStackBasePointer)), callFrameReturnStackBasePointerOffset)
	require.Equal(t, int(unsafe.Offsetof(frame.compiledFunction)), callFrameCompiledFunctionOffset)

	// Offsets for compiledFunction.
	var compiledFunc compiledFunction
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.codeInitialAddress)), compiledFunctionCodeInitialAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.maxStackPointer)), compiledFunctionMaxStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.moduleInstanceAddress)), compiledFunctionModuleInstanceAddressOffset)

	// Offsets for wasm.TableElement.
	var tableElement wasm.TableElement
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionAddress)), tableElementFunctionAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionTypeID)), tableElementFunctionTypeIDOffset)

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Memory)), moduleInstanceMemoryOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Tables)), moduleInstanceTablesOffset)

	// Offsets for wasm.TableInstance.
	var tableInstance wasm.TableInstance
	require.Equal(t, int(unsafe.Offsetof(tableInstance.Table)), tableInstanceTableOffset)

	// Offsets for wasm.MemoryInstance
	var memoryInstance wasm.MemoryInstance
	require.Equal(t, int(unsafe.Offsetof(memoryInstance.Buffer)), memoryInstanceBufferOffset)

	// Offsets for wasm.GlobalInstance
	var globalInstance wasm.GlobalInstance
	require.Equal(t, int(unsafe.Offsetof(globalInstance.Val)), globalInstanceValueOffset)
}

func Test_Simple(t *testing.T) {
	mod, err := text.DecodeModule([]byte(`(module
	(import "" "hello" (func $hello))
	(start $hello)
)`))
	require.NoError(t, err)

	engine := newEngine()
	store := wasm.NewStore(engine)

	msg := "hello!"
	hostFunction := func(ctx *wasm.HostFunctionCallContext) {
		require.NotNil(t, ctx.Memory)
		copy(ctx.Memory.Buffer, msg)
	}
	require.NoError(t, store.AddHostFunction("", "hello", reflect.ValueOf(hostFunction)))

	memoryInstance := &wasm.MemoryInstance{Buffer: make([]byte, len(msg))}
	engine.compiledFunctions[0].source.ModuleInstance.Memory = memoryInstance

	moduleName := "simple"
	require.NoError(t, store.Instantiate(mod, moduleName))

	// The "hello" function was imported as $hello in Wasm. Since it was marked as the start
	// function, it is invoked on instantiation. Ensure that worked: "hello" was called!
	require.Equal(t, msg, string(memoryInstance.Buffer))
}

func TestEngine_fibonacci(t *testing.T) {
	buf, err := os.ReadFile("testdata/fib.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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
	mod, err := binary.DecodeModule(buf)
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
	require.ErrorIs(t, err, wasm.ErrRuntimeCallStackOverflow)
}

func TestEngine_unreachable(t *testing.T) {
	buf, err := os.ReadFile("testdata/unreachable.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)
	store := wasm.NewStore(NewEngine())
	require.NoError(t, err)

	const moduleName = "test"

	callUnreachable := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction(moduleName, "unreachable_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("host", "cause_unreachable", reflect.ValueOf(callUnreachable))
	require.NoError(t, err)

	err = store.Instantiate(mod, moduleName)
	require.NoError(t, err)

	_, _, err = store.CallFunction(moduleName, "main")
	exp := `wasm runtime error: unreachable
wasm backtrace:
	0: unreachable_func
	1: host.cause_unreachable
	2: two
	3: one
	4: main`
	require.ErrorIs(t, err, wasm.ErrRuntimeUnreachable)
	require.Equal(t, exp, err.Error())
}

func TestEngine_memory(t *testing.T) {
	buf, err := os.ReadFile("testdata/memory.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
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

func TestEngine_RecursiveEntry(t *testing.T) {
	buf, err := os.ReadFile("testdata/recursive.wasm")
	require.NoError(t, err)
	mod, err := binary.DecodeModule(buf)
	require.NoError(t, err)

	eng := newEngine()
	store := wasm.NewStore(eng)

	hostfunc := func(ctx *wasm.HostFunctionCallContext) {
		_, _, err := store.CallFunction("test", "called_by_host_func")
		require.NoError(t, err)
	}
	err = store.AddHostFunction("env", "host_func", reflect.ValueOf(hostfunc))
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "main", uint64(1))
	require.NoError(t, err)
}
