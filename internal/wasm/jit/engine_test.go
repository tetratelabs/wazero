package jit

import (
	"context"
	"math"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// Ensures that the offset consts do not drift when we manipulate the target structs.
func TestVerifyOffsetValue(t *testing.T) {
	var vm callEngine
	// Offsets for callEngine.globalContext.
	require.Equal(t, int(unsafe.Offsetof(vm.valueStackElement0Address)), callEngineGlobalContextValueStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.valueStackLen)), callEngineGlobalContextValueStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.callFrameStackElementZeroAddress)), callEngineGlobalContextCallFrameStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.callFrameStackLen)), callEngineGlobalContextCallFrameStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.callFrameStackPointer)), callEngineGlobalContextCallFrameStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.compiledFunctionsElement0Address)), callEngineGlobalContextCompiledFunctionsElement0AddressOffset)

	// Offsets for callEngine.moduleContext.
	require.Equal(t, int(unsafe.Offsetof(vm.moduleInstanceAddress)), callEngineModuleContextModuleInstanceAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.globalElement0Address)), callEngineModuleContextGlobalElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.memoryElement0Address)), callEngineModuleContextMemoryElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.memorySliceLen)), callEngineModuleContextMemorySliceLenOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.tableElement0Address)), callEngineModuleContextTableElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.tableSliceLen)), callEngineModuleContextTableSliceLenOffset)

	// Offsets for callEngine.valueStackContext
	require.Equal(t, int(unsafe.Offsetof(vm.stackPointer)), callEngineValueStackContextStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.stackBasePointer)), callEngineValueStackContextStackBasePointerOffset)

	// Offsets for callEngine.exitContext.
	require.Equal(t, int(unsafe.Offsetof(vm.statusCode)), callEngineExitContextJITCallStatusCodeOffset)
	require.Equal(t, int(unsafe.Offsetof(vm.functionCallAddress)), callEngineExitContextFunctionCallAddressOffset)

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
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.stackPointerCeil)), compiledFunctionStackPointerCeilOffset)

	// Offsets for wasm.TableElement.
	var tableElement wasm.TableElement
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionIndex)), tableElementFunctionIndexOffset)
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionTypeID)), tableElementFunctionTypeIDOffset)

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.MemoryInstance)), moduleInstanceMemoryOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.TableInstance)), moduleInstanceTableOffset)

	// Offsets for wasm.TableInstance.
	var tableInstance wasm.TableInstance
	require.Equal(t, int(unsafe.Offsetof(tableInstance.Table)), tableInstanceTableOffset)
	// We add "+8" to get the length of Tables[0].Table
	// since the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	require.Equal(t, int(unsafe.Offsetof(tableInstance.Table)+8), tableInstanceTableLenOffset)

	// Offsets for wasm.MemoryInstance
	var memoryInstance wasm.MemoryInstance
	require.Equal(t, int(unsafe.Offsetof(memoryInstance.Buffer)), memoryInstanceBufferOffset)
	// "+8" because the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	require.Equal(t, int(unsafe.Offsetof(memoryInstance.Buffer)+8), memoryInstanceBufferLenOffset)

	// Offsets for wasm.GlobalInstance
	var globalInstance wasm.GlobalInstance
	require.Equal(t, int(unsafe.Offsetof(globalInstance.Val)), globalInstanceValueOffset)
}

func TestEngine_Call(t *testing.T) {
	requireSupportedOSArch(t)

	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
	}

	// Use exported functions to simplify instantiation of a Wasm function
	e := NewEngine()
	store := wasm.NewStore(context.Background(), e)
	_, err := store.Instantiate(m, "")
	require.NoError(t, err)

	// ensure base case doesn't fail
	results, err := e.Call(store.ModuleContexts[""], store.Functions[0], 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := e.Call(store.ModuleContexts[""], store.Functions[0])
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := e.Call(store.ModuleContexts[""], store.Functions[0], 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func TestEngine_Call_HostFn(t *testing.T) {
	requireSupportedOSArch(t)

	memory := &wasm.MemoryInstance{}
	var ctxMemory publicwasm.Memory
	hostFn := reflect.ValueOf(func(ctx publicwasm.ModuleContext, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := NewEngine()
	module := &wasm.ModuleInstance{MemoryInstance: memory}
	modCtx := wasm.NewModuleContext(context.Background(), e, module)
	f := &wasm.FunctionInstance{
		HostFunction: &hostFn,
		FunctionKind: wasm.FunctionKindGoModuleContext,
		FunctionType: &wasm.TypeInstance{
			Type: &wasm.FunctionType{
				Params:  []wasm.ValueType{wasm.ValueTypeI64},
				Results: []wasm.ValueType{wasm.ValueTypeI64},
			},
		},
		ModuleInstance: module,
	}
	require.NoError(t, e.Compile(f))

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := e.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := e.Call(modCtx, f)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := e.Call(modCtx, f, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func requireSupportedOSArch(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}
	if runtime.GOOS == "windows" { // TODO: #269
		t.Skip()
	}
}
