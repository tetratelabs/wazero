package jit

import (
	"math"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
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
	require.Equal(t, int(unsafe.Offsetof(eng.previousCallFrameStackPointer)), engineGlobalContextPreviousCallFrameStackPointer)
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
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.stackPointerCeil)), compiledFunctionStackPointerCeilOffset)

	// Offsets for wasm.TableElement.
	var tableElement wasm.TableElement
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionAddress)), tableElementFunctionAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(tableElement.FunctionTypeID)), tableElementFunctionTypeIDOffset)

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.MemoryInstance)), moduleInstanceMemoryOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Tables)), moduleInstanceTablesOffset)

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
