package jit

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Ensures that the offset consts do not drift when we manipulate the target structs.
func TestJIT_VerifyOffsetValue(t *testing.T) {
	var me moduleEngine
	require.Equal(t, int(unsafe.Offsetof(me.compiledFunctions)), moduleEngineCompiledFunctionsOffset)

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
	require.Equal(t, int(unsafe.Offsetof(ce.tableElement0Address)), callEngineModuleContextTableElement0AddressOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.tableSliceLen)), callEngineModuleContextTableSliceLenOffset)
	require.Equal(t, int(unsafe.Offsetof(ce.compiledFunctionsElement0Address)), callEngineModuleContextCompiledFunctionsElement0AddressOffset)

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
	require.Equal(t, int(unsafe.Offsetof(frame.compiledFunction)), callFrameCompiledFunctionOffset)

	// Offsets for compiledFunction.
	var compiledFunc compiledFunction
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.codeInitialAddress)), compiledFunctionCodeInitialAddressOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.stackPointerCeil)), compiledFunctionStackPointerCeilOffset)
	require.Equal(t, int(unsafe.Offsetof(compiledFunc.source)), compiledFunctionSourceOffset)

	// Offsets for wasm.ModuleInstance.
	var moduleInstance wasm.ModuleInstance
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Globals)), moduleInstanceGlobalsOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Memory)), moduleInstanceMemoryOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Table)), moduleInstanceTableOffset)
	require.Equal(t, int(unsafe.Offsetof(moduleInstance.Engine)), moduleInstanceEngineOffset)

	var functionInstance wasm.FunctionInstance
	require.Equal(t, int(unsafe.Offsetof(functionInstance.TypeID)), functionInstanceTypeIDOffset)

	// Offsets for wasm.Table.
	var tableInstance wasm.TableInstance
	require.Equal(t, int(unsafe.Offsetof(tableInstance.Table)), tableInstanceTableOffset)
	// We add "+8" to get the length of Tables[0].Table
	// since the slice header is laid out as {Data uintptr, Len int64, Cap int64} on memory.
	require.Equal(t, int(unsafe.Offsetof(tableInstance.Table)+8), tableInstanceTableLenOffset)

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
}

// et is used for tests defined in the enginetest package.
var et = &engineTester{}

type engineTester struct{}

func (e *engineTester) NewEngine() wasm.Engine {
	return newEngine()
}

func (e *engineTester) InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{} {
	table := make([]interface{}, initTableLen)
	internal := me.(*moduleEngine)
	for idx, fnidx := range initTableIdxToFnIdx {
		table[idx] = internal.compiledFunctions[fnidx]
	}
	return table
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

func requireSupportedOSArch(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}
}

func TestJIT_EngineCompile_Errors(t *testing.T) {
	t.Run("invalid import", func(t *testing.T) {
		e := et.NewEngine()
		_, err := e.NewModuleEngine(
			t.Name(),
			[]*wasm.FunctionInstance{{Module: &wasm.ModuleInstance{Name: "uncompiled"}, DebugName: "uncompiled.fn"}},
			nil, // moduleFunctions
			nil, // table
			nil, // tableInit
		)
		require.EqualError(t, err, "import[0] func[uncompiled.fn]: uncompiled")
	})

	t.Run("release on compilation error", func(t *testing.T) {
		e := et.NewEngine().(*engine)

		importedFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		_, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)

		require.Len(t, e.compiledFunctions, len(importedFunctions))

		moduleFunctions := []*wasm.FunctionInstance{
			{DebugName: "ok1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "ok2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "invalid code", Type: &wasm.FunctionType{}, Body: []byte{
				wasm.OpcodeCall, // Call instruction without immediate for call target index is invalid and should fail to compile.
			}, Module: &wasm.ModuleInstance{}},
		}

		_, err = e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, nil, nil)
		require.EqualError(t, err, "function[2/2] failed to lower to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, all the compiled functions including succeeded ones must be released.
		require.Len(t, e.compiledFunctions, len(importedFunctions))
		for _, f := range moduleFunctions {
			require.NotContains(t, e.compiledFunctions, f)
		}
	})
}

type fakeFinalizer map[*compiledFunction]func(*compiledFunction)

func (f fakeFinalizer) setFinalizer(obj interface{}, finalizer interface{}) {
	cf := obj.(*compiledFunction)
	if _, ok := f[cf]; ok { // easier than adding a field for testing.T
		panic(fmt.Sprintf("BUG: %v already had its finalizer set", cf))
	}
	f[cf] = finalizer.(func(*compiledFunction))
}

func TestJIT_NewModuleEngine_CompiledFunctions(t *testing.T) {
	newFunctionInstance := func(id int) *wasm.FunctionInstance {
		return &wasm.FunctionInstance{
			DebugName: strconv.Itoa(id),
			Type:      &wasm.FunctionType{},
			Body:      []byte{wasm.OpcodeEnd},
			Module:    &wasm.ModuleInstance{},
		}
	}

	e := et.NewEngine().(*engine)

	importedFinalizer := fakeFinalizer{}
	e.setFinalizer = importedFinalizer.setFinalizer

	importedFunctions := []*wasm.FunctionInstance{
		newFunctionInstance(10),
		newFunctionInstance(20),
	}
	modE, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
	require.NoError(t, err)
	defer modE.CloseWithExitCode(0) //nolint
	imported := modE.(*moduleEngine)

	importingFinalizer := fakeFinalizer{}
	e.setFinalizer = importingFinalizer.setFinalizer

	moduleFunctions := []*wasm.FunctionInstance{
		newFunctionInstance(100),
		newFunctionInstance(200),
		newFunctionInstance(300),
	}

	modE, err = e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, nil, nil)
	require.NoError(t, err)
	defer modE.CloseWithExitCode(0) //nolint
	importing := modE.(*moduleEngine)

	// Ensure the importing module didn't try to finalize the imported functions.
	require.Equal(t, len(importedFunctions), len(imported.compiledFunctions))
	for _, f := range importedFunctions {
		require.Contains(t, e.compiledFunctions, f)
		cf := e.compiledFunctions[f]
		require.Contains(t, importedFinalizer, cf)
		require.NotContains(t, importingFinalizer, cf)
	}

	// The importing module's compiled functions include ones it compiled (module-defined) and imported ones).
	require.Equal(t, len(importedFunctions)+len(moduleFunctions), len(importing.compiledFunctions))

	// Ensure the importing module only tried to finalize its own functions.
	for _, f := range moduleFunctions {
		require.Contains(t, e.compiledFunctions, f)
		cf := e.compiledFunctions[f]
		require.NotContains(t, importedFinalizer, cf)
		require.Contains(t, importingFinalizer, cf)
	}

	// Pretend the finalizer executed, by invoking them one-by-one.
	for k, v := range importingFinalizer {
		v(k)
	}
	for k, v := range importedFinalizer {
		v(k)
	}
	for _, f := range e.compiledFunctions {
		require.Nil(t, f.codeSegment) // Set to nil if the correct finalizer was associated.
	}
}

// TestReleaseCompiledFunction_Panic tests that an unexpected panic has some identifying information in it.
func TestJIT_ReleaseCompiledFunction_Panic(t *testing.T) {
	// capturePanic because there's no require.PanicsWithErrorPrefix
	errMessage := capturePanic(func() {
		releaseCompiledFunction(&compiledFunction{
			codeSegment: []byte{wasm.OpcodeEnd},                                                         // never compiled means it was never mapped.
			source:      &wasm.FunctionInstance{Index: 2, Module: &wasm.ModuleInstance{Name: t.Name()}}, // for error string
		})
	})
	require.Contains(t, errMessage.Error(),
		fmt.Sprintf("jit: failed to munmap code segment for %[1]s.function[2]:", t.Name()))
}

// capturePanic returns an error recovered from a panic
func capturePanic(panics func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if e, ok := recovered.(error); ok {
				err = e
			}
		}
	}()
	panics()
	return
}

func TestJIT_ModuleEngine_Close(t *testing.T) {
	newFunctionInstance := func(id int) *wasm.FunctionInstance {
		return &wasm.FunctionInstance{
			DebugName: strconv.Itoa(id), Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}}
	}

	for _, tc := range []struct {
		name                               string
		importedFunctions, moduleFunctions []*wasm.FunctionInstance
	}{
		{
			name:            "no imports",
			moduleFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
		},
		{
			name:              "only imports",
			importedFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
		},
		{
			name:              "mix",
			importedFunctions: []*wasm.FunctionInstance{newFunctionInstance(0), newFunctionInstance(1)},
			moduleFunctions:   []*wasm.FunctionInstance{newFunctionInstance(100), newFunctionInstance(200), newFunctionInstance(300)},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			e := et.NewEngine().(*engine)
			var imported *moduleEngine
			if len(tc.importedFunctions) > 0 {
				// Instantiate the imported module
				modEngine, err := e.NewModuleEngine(
					fmt.Sprintf("%s - imported functions", t.Name()),
					nil, // moduleFunctions
					tc.importedFunctions,
					nil, // table
					nil, // tableInit
				)
				require.NoError(t, err)
				imported = modEngine.(*moduleEngine)
				require.Len(t, imported.compiledFunctions, len(tc.importedFunctions))
			}

			importing, err := e.NewModuleEngine(
				fmt.Sprintf("%s - module-defined functions", t.Name()),
				tc.importedFunctions,
				tc.moduleFunctions,
				nil, // table
				nil, // tableInit
			)
			require.NoError(t, err)
			require.Len(t, importing.(*moduleEngine).compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for _, f := range tc.moduleFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}

			closed, err := importing.CloseWithExitCode(0)
			require.True(t, closed)
			require.NoError(t, err)

			// Closing should flip the status bit, so that it cannot be closed again.
			require.Equal(t, uint64(1), importing.(*moduleEngine).closed)

			// Closing the importing module shouldn't delete the imported functions from the engine.
			require.Len(t, e.compiledFunctions, len(tc.importedFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}

			// However, closing the importing module should delete its own functions from the engine.
			for i, f := range tc.moduleFunctions {
				require.NotContains(t, e.compiledFunctions, f, i)
			}

			if len(tc.importedFunctions) > 0 {
				closed, err = imported.CloseWithExitCode(0)
				require.True(t, closed)
				require.NoError(t, err)
			}

			// When all modules are closed, the engine should be empty.
			require.Empty(t, e.compiledFunctions)
		})
	}
}

// Ensures that value stack and call-frame stack are allocated on heap which
// allows us to safely access to their data region from native code.
// See comments on initialValueStackSize and initialCallFrameStackSize.
func TestJIT_SliceAllocatedOnHeap(t *testing.T) {
	e := newEngine()
	store := wasm.NewStore(e, wasm.Features20191205)

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
	}})
	require.NoError(t, err)

	_, err = store.Instantiate(context.Background(), hm, hostModuleName, nil)
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
		ExportSection: map[string]*wasm.Export{
			valueStackCorruption: {Type: wasm.ExternTypeFunc, Index: 1, Name: valueStackCorruption},
			callStackCorruption:  {Type: wasm.ExternTypeFunc, Index: 2, Name: callStackCorruption},
		},
	}

	mi, err := store.Instantiate(context.Background(), m, t.Name(), nil)
	require.NoError(t, err)

	for _, fnName := range []string{valueStackCorruption, callStackCorruption} {
		fnName := fnName
		t.Run(fnName, func(t *testing.T) {
			ret, err := mi.ExportedFunction(fnName).Call(nil)
			require.NoError(t, err)

			require.Equal(t, uint32(expectedReturnValue), uint32(ret[0]))
		})
	}
}
