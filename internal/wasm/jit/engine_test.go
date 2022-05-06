package jit

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// Ensures that the offset consts do not drift when we manipulate the target structs.
func TestJIT_VerifyOffsetValue(t *testing.T) {
	var me moduleEngine
	require.Equal(t, int(unsafe.Offsetof(me.functions)), moduleEngineFunctionsOffset)

	var ce callEngine
	// Offsets for callEngine.globalContext.
	require.Equal(t, int(unsafe.Offsetof(ce.valueStackElement0Address)), callEngineGlobalContextValueStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.valueStackLen)), callEngineGlobalContextValueStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.callFrameStackElementZeroAddress)), callEngineGlobalContextCallFrameStackElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.callFrameStackLen)), callEngineGlobalContextCallFrameStackLenOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.callFrameStackPointer)), callEngineGlobalContextCallFrameStackPointerOffset)

	// Offsets for callEngine.moduleContext.
	require.Equal(t, int(unsafe.Offsetof(ce.moduleInstanceAddress)), callEngineModuleContextModuleInstanceAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.globalElement0Address)), callEngineModuleContextGlobalElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.memoryElement0Address)), callEngineModuleContextMemoryElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.memorySliceLen)), callEngineModuleContextMemorySliceLenOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.tablesElement0Address)), callEngineModuleContextTablesElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.codesElement0Address)), callEngineModuleContextCodesElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.typeIDsElement0Address)), callEngineModuleContextTypeIDsElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.dataInstancesElement0Address)), callEngineModuleContextDataInstancesElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.elementInstancesElemen0Address)), callEngineModuleContextElementInstancesElement0AddressOffset)

	// Offsets for callEngine.valueStackContext
	require.Equal(t, int(unsafe.Offsetof(ce.stackPointer)), callEngineValueStackContextStackPointerOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.stackBasePointer)), callEngineValueStackContextStackBasePointerOffset)

	// Offsets for callEngine.exitContext.
	require.Equal(t, int(unsafe.Offsetof(ce.statusCode)), callEngineExitContextJITCallStatusCodeOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.builtinFunctionCallIndex)), callEngineExitContextBuiltinFunctionCallAddressOffset)

	// Size and offsets for callFrame.
	var frame callFrame
	require.Equal(t, int(unsafe.Sizeof(frame)), callFrameDataSize)
	// Sizeof call-frame must be a power of 2 as we do SHL on the index by "callFrameDataSizeMostSignificantSetBit" to obtain the offset address.
	require.True(t, callFrameDataSize&(callFrameDataSize-1) == 0)
	require.Equal(t, math.Ilogb(float64(callFrameDataSize)), callFrameDataSizeMostSignificantSetBit)
	require.Equal(t, int(unsafe.Offsetof(frame.returnAddress)), callFrameReturnAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(frame.returnStackBasePointer)), callFrameReturnStackBasePointerOffset)
	require.Equal(t, int(unsafe.Offsetof(frame.function)), callFrameFunctionOffset)

	// Offsets for code.
	var compiledFunc function
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.codeInitialAddress)), functionCodeInitialAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.stackPointerCeil)), functionStackPointerCeilOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.source)), functionSourceOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.moduleInstanceAddress)), functionModuleInstanceAddressOffset)

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Memory)), moduleInstanceMemoryOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Tables)), moduleInstanceTablesOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Engine)), moduleInstanceEngineOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.TypeIDs)), moduleInstanceTypeIDsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.DataInstances)), moduleInstanceDataInstancesOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.ElementInstances)), moduleInstanceElementInstancesOffset)

	var functionInstance wasm.FunctionInstance
	require.Equal(t, int(unsafe.Offsetof(functionInstance.TypeID)), functionInstanceTypeIDOffset)

	// Offsets for wasm.Table.
	var tableInstance wasm.TableInstance
	require.Equal(t, int(unsafe.Offsetof(tableInstance.References)), tableInstanceTableOffset)
	// We add "+8" to get the length of Tables[0].Table
	// since the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	require.Equal(t, int(unsafe.Offsetof(tableInstance.References)+8), tableInstanceTableLenOffset)

	// Offsets for wasm.Memory
	var memoryInstance wasm.MemoryInstance
	require.Equal(t, int(unsafe.Offsetof(memoryInstance.Buffer)), memoryInstanceBufferOffset)
	// "+8" because the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	require.Equal(t, int(unsafe.Offsetof(memoryInstance.Buffer)+8), memoryInstanceBufferLenOffset)

	// Offsets for wasm.GlobalInstance
	var globalInstance wasm.GlobalInstance
	require.Equal(t, int(unsafe.Offsetof(globalInstance.Val)), globalInstanceValueOffset)

	// Offsets for Go's interface.
	// The underlying struct is not exposed in the public API, so we simulate it here.
	// https://github.com/golang/go/blob/release-branch.go1.17/src/runtime/runtime2.go#L207-L210
	var eface struct {
		_type *struct{}
		data  unsafe.Pointer
	}
	require.Equal(t, int(unsafe.Offsetof(eface.data)), interfaceDataOffset)
	require.Equal(t, int(unsafe.Sizeof(eface)), 1<<interfaceDataSizeLog2)

	var dataInstance wasm.DataInstance
	require.Equal(t, int(unsafe.Sizeof(dataInstance)), dataInstanceStructSize)

	var elementInstance wasm.ElementInstance
	require.Equal(t, int(unsafe.Sizeof(elementInstance)), elementInstanceStructSize)
}

// et is used for tests defined in the enginetest package.
var et = &engineTester{}

// engineTester implements enginetest.EngineTester.
type engineTester struct{}

// NewEngine implements enginetest.EngineTester NewEngine.
func (e *engineTester) NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return newEngine(enabledFeatures)
}

// InitTables implements enginetest.EngineTester InitTables.
func (e engineTester) InitTables(me wasm.ModuleEngine, tableIndexToLen map[wasm.Index]int, initTableIdxToFnIdx wasm.TableInitMap) [][]wasm.Reference {
	references := make([][]wasm.Reference, len(tableIndexToLen))
	for tableIndex, l := range tableIndexToLen {
		references[tableIndex] = make([]interface{}, l)
	}
	internal := me.(*moduleEngine)

	for tableIndex, init := range initTableIdxToFnIdx {
		referencesPerTable := references[tableIndex]
		for idx, fnidx := range init {
			referencesPerTable[idx] = internal.functions[fnidx]
		}
	}
	return references
}

func TestJIT_Engine_NewModuleEngine(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestEngine_NewModuleEngine(t, et)
}

func TestJIT_Engine_NewModuleEngine_InitTable(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestEngine_NewModuleEngine_InitTable(t, et)
}

func TestJIT_ModuleEngine_Call(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call(t, et)
}

func TestJIT_ModuleEngine_Call_HostFn(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call_HostFn(t, et)
}

func TestJIT_ModuleEngine_Call_Errors(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call_Errors(t, et)
}

func TestJIT_ModuleEngine_Memory(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Memory(t, et)
}

func requireSupportedOSArch(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}
}

type fakeFinalizer map[*code]func(*code)

func (f fakeFinalizer) setFinalizer(obj interface{}, finalizer interface{}) {
	cf := obj.(*code)
	if _, ok := f[cf]; ok { // easier than adding a field for testing.T
		panic(fmt.Sprintf("BUG: %v already had its finalizer set", cf))
	}
	f[cf] = finalizer.(func(*code))
}

func TestJIT_CompileModule(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		e := et.NewEngine(wasm.Features20191205).(*engine)
		ff := fakeFinalizer{}
		e.setFinalizer = ff.setFinalizer

		okModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{},
		}

		err := e.CompileModule(testCtx, okModule)
		require.NoError(t, err)

		// Compiling same module shouldn't be compiled again, but instead should be cached.
		err = e.CompileModule(testCtx, okModule)
		require.NoError(t, err)

		compiled, ok := e.codes[okModule.ID]
		require.True(t, ok)
		require.Equal(t, len(okModule.FunctionSection), len(compiled))

		// Pretend the finalizer executed, by invoking them one-by-one.
		for k, v := range ff {
			v(k)
		}
	})

	t.Run("fail", func(t *testing.T) {
		errModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeCall}}, // Call instruction without immediate for call target index is invalid and should fail to compile.
			},
			ID: wasm.ModuleID{},
		}

		e := et.NewEngine(wasm.Features20191205).(*engine)
		err := e.CompileModule(testCtx, errModule)
		require.EqualError(t, err, "failed to lower func[2/3] to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, the compiled functions must not be cached.
		_, ok := e.codes[errModule.ID]
		require.False(t, ok)
	})
}

// TestJIT_Releasecode_Panic tests that an unexpected panic has some identifying information in it.
func TestJIT_Releasecode_Panic(t *testing.T) {
	captured := require.CapturePanic(func() {
		releaseCode(&code{
			indexInModule: 2,
			sourceModule:  &wasm.Module{NameSection: &wasm.NameSection{ModuleName: t.Name()}},
			codeSegment:   []byte{wasm.OpcodeEnd}, // never compiled means it was never mapped.
		})
	})
	require.Contains(t, captured.Error(), fmt.Sprintf("jit: failed to munmap code segment for %[1]s.function[2]", t.Name()))
}

// Ensures that value stack and call-frame stack are allocated on heap which
// allows us to safely access to their data region from native code.
// See comments on initialValueStackSize and initialCallFrameStackSize.
func TestJIT_SliceAllocatedOnHeap(t *testing.T) {
	enabledFeatures := wasm.Features20191205
	e := newEngine(enabledFeatures)
	store := wasm.NewStore(enabledFeatures, e)

	const hostModuleName = "env"
	const hostFnName = "grow_and_shrink_goroutine_stack"
	hm, err := wasm.NewHostModule(hostModuleName, map[string]interface{}{hostFnName: func() {
		// This function aggressively grow the goroutine stack by recursively
		// calling the function many times.
		var callNum = 1000
		var growGoroutineStack func()
		growGoroutineStack = func() {
			if callNum != 0 {
				callNum--
				growGoroutineStack()
			}
		}
		growGoroutineStack()

		// Trigger relocation of goroutine stack because at this point we have the majority of
		// goroutine stack unused after recursive call.
		runtime.GC()
	}}, map[string]*wasm.Memory{}, map[string]*wasm.Global{}, enabledFeatures)
	require.NoError(t, err)

	err = store.Engine.CompileModule(testCtx, hm)
	require.NoError(t, err)

	_, err = store.Instantiate(testCtx, hm, hostModuleName, nil, nil)
	require.NoError(t, err)

	const valueStackCorruption = "value_stack_corruption"
	const callStackCorruption = "call_stack_corruption"
	const expectedReturnValue = 0x1
	m := &wasm.Module{
		TypeSection: []*wasm.FunctionType{
			{Params: []wasm.ValueType{}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
			{Params: []wasm.ValueType{}, Results: []wasm.ValueType{}},
		},
		FunctionSection: []wasm.Index{
			wasm.Index(0),
			wasm.Index(0),
			wasm.Index(0),
		},
		CodeSection: []*wasm.Code{
			{
				// value_stack_corruption
				Body: []byte{
					wasm.OpcodeCall, 0, // Call host function to shrink Goroutine stack
					// We expect this value is returned, but if the stack is allocated on
					// goroutine stack, we write this expected value into the old-location of
					// stack.
					wasm.OpcodeI32Const, expectedReturnValue,
					wasm.OpcodeEnd,
				},
			},
			{
				// call_stack_corruption
				Body: []byte{
					wasm.OpcodeCall, 3, // Call the wasm function below.
					// At this point, call stack's memory looks like [call_stack_corruption, index3]
					// With this function call it should end up [call_stack_corruption, host func]
					// but if the call-frame stack is allocated on goroutine stack, we exit the native code
					// with  [call_stack_corruption, index3] (old call frame stack) with HostCall status code,
					// and end up trying to call index3 as a host function which results in nil pointer exception.
					wasm.OpcodeCall, 0,
					wasm.OpcodeI32Const, expectedReturnValue,
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
		},
		ImportSection: []*wasm.Import{{Module: hostModuleName, Name: hostFnName, DescFunc: 1}},
		ExportSection: []*wasm.Export{
			{Type: wasm.ExternTypeFunc, Index: 1, Name: valueStackCorruption},
			{Type: wasm.ExternTypeFunc, Index: 2, Name: callStackCorruption},
		},
		ID: wasm.ModuleID{1},
	}

	err = store.Engine.CompileModule(testCtx, m)
	require.NoError(t, err)

	mi, err := store.Instantiate(testCtx, m, t.Name(), nil, nil)
	require.NoError(t, err)

	for _, fnName := range []string{valueStackCorruption, callStackCorruption} {
		fnName := fnName
		t.Run(fnName, func(t *testing.T) {
			ret, err := mi.ExportedFunction(fnName).Call(testCtx)
			require.NoError(t, err)

			require.Equal(t, uint32(expectedReturnValue), uint32(ret[0]))
		})
	}
}

// TODO: move most of this logic to enginetest.go so that there is less drift between interpreter and jit
func TestEngine_Cachedcodes(t *testing.T) {
	e := newEngine(wasm.Features20191205)
	exp := []*code{
		{codeSegment: []byte{0x0}},
		{codeSegment: []byte{0x0}},
	}
	m := &wasm.Module{}

	e.addCodes(m, exp)

	actual, ok := e.getCodes(m)
	require.True(t, ok)
	require.Equal(t, len(exp), len(actual))
	for i := range actual {
		require.Equal(t, exp[i], actual[i])
	}

	e.deleteCodes(m)
	_, ok = e.getCodes(m)
	require.False(t, ok)
}
