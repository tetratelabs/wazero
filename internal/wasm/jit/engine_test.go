package jit

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// Ensures that the offset consts do not drift when we manipulate the target structs.
func TestVerifyOffsetValue(t *testing.T) {
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

func TestEngine_NewModuleEngine(t *testing.T) {
	requireSupportedOSArch(t)

	e := NewEngine()

	t.Run("sets module name", func(t *testing.T) {
		me, err := e.NewModuleEngine(t.Name(), nil, nil, nil, nil)
		require.NoError(t, err)
		defer me.Close()
		require.Equal(t, t.Name(), me.(*moduleEngine).name)
	})
}

func TestEngine_Call(t *testing.T) {
	requireSupportedOSArch(t)

	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []wasm.Index{wasm.Index(0)},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
		ExportSection:   map[string]*wasm.Export{"fn": {Type: wasm.ExternTypeFunc, Index: 0, Name: "fn"}},
	}

	// Use exported functions to simplify instantiation of a Wasm function
	e := NewEngine()
	store := wasm.NewStore(e, wasm.Features20191205)
	mod, err := store.Instantiate(context.Background(), m, t.Name())
	require.NoError(t, err)

	fn := mod.ExportedFunction("fn")
	require.NotNil(t, fn)

	// ensure base case doesn't fail
	results, err := fn.Call(nil, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := fn.Call(nil)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := fn.Call(nil, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func TestEngine_NewModuleEngine_InitTable(t *testing.T) {
	e := NewEngine()

	t.Run("no table elements", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		var moduleFunctions []*wasm.FunctionInstance
		var tableInit map[wasm.Index]wasm.Index

		// Instantiate the module, which has nothing but an empty table.
		me, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// Since there are no elements to initialize, we expect the table to be nil.
		require.Equal(t, table.Table, make([]interface{}, 2))

		// Clean up.
		require.NoError(t, me.Close())
	})

	t.Run("module-defined function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		moduleFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		tableInit := map[wasm.Index]wasm.Index{0: 2}

		// Instantiate the module whose table points to its own functions.
		me, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// The functions mapped to the table are defined in the same moduleEngine
		require.Equal(t, table.Table, []interface{}{me.(*moduleEngine).compiledFunctions[2], nil})

		// Clean up.
		require.NoError(t, me.Close())
	})

	t.Run("imported function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		importedFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		var moduleFunctions []*wasm.FunctionInstance
		tableInit := map[wasm.Index]wasm.Index{0: 2}

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets is absolute.
		require.Equal(t, table.Table, []interface{}{importing.(*moduleEngine).compiledFunctions[2], nil})

		// Clean up.
		require.NoError(t, importing.Close())
		require.NoError(t, imported.Close())
	})

	t.Run("mixed functions", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		importedFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		moduleFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		tableInit := map[wasm.Index]wasm.Index{0: 0, 1: 4}

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets are absolute.
		require.Equal(t, table.Table, []interface{}{
			importing.(*moduleEngine).compiledFunctions[0],
			importing.(*moduleEngine).compiledFunctions[4],
		})

		// Clean up.
		require.NoError(t, importing.Close())
		require.NoError(t, imported.Close())
	})
}

func TestEngine_Call_HostFn(t *testing.T) {
	requireSupportedOSArch(t)

	memory := &wasm.MemoryInstance{}
	var ctxMemory publicwasm.Memory
	hostFn := reflect.ValueOf(func(ctx publicwasm.Module, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := NewEngine()
	module := &wasm.ModuleInstance{Memory: memory}
	modCtx := wasm.NewModuleContext(context.Background(), wasm.NewStore(e, wasm.Features20191205), module)
	f := &wasm.FunctionInstance{
		GoFunc: &hostFn,
		Kind:   wasm.FunctionKindGoModule,
		Type: &wasm.FunctionType{
			Params:  []wasm.ValueType{wasm.ValueTypeI64},
			Results: []wasm.ValueType{wasm.ValueTypeI64},
		},
		Module: module,
	}

	modEngine, err := e.NewModuleEngine(t.Name(), nil, []*wasm.FunctionInstance{f}, nil, nil)
	require.NoError(t, err)

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := modEngine.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := modEngine.Call(modCtx, f)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := modEngine.Call(modCtx, f, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func requireSupportedOSArch(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}
}

func TestEngineCompile_Errors(t *testing.T) {
	t.Run("invalid import", func(t *testing.T) {
		e := newEngine()
		_, err := e.NewModuleEngine(
			t.Name(),
			[]*wasm.FunctionInstance{{Module: &wasm.ModuleInstance{Name: "uncompiled"}, Name: "fn"}},
			nil, // moduleFunctions
			nil, // table
			nil, // tableInit
		)
		require.EqualError(t, err, "import[0] func[uncompiled.fn]: uncompiled")
	})

	t.Run("release on compilation error", func(t *testing.T) {
		e := newEngine()

		importedFunctions := []*wasm.FunctionInstance{
			{Name: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		_, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)

		require.Len(t, e.compiledFunctions, len(importedFunctions))

		moduleFunctions := []*wasm.FunctionInstance{
			{Name: "ok1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "ok2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "invalid code", Type: &wasm.FunctionType{}, Body: []byte{
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

var fakeFinalizer = func(obj interface{}, finalizer interface{}) {
	cf := obj.(*compiledFunction)
	fn := finalizer.(func(compiledFn *compiledFunction))
	fn(cf)
}

// TestModuleEngine_Close_Panic tests that an unexpected panic has some identifying information in it.
func TestModuleEngine_Close_Panic(t *testing.T) {
	e := newEngine()
	me := &moduleEngine{
		name: t.Name(),
		compiledFunctions: []*compiledFunction{
			{codeSegment: []byte{wasm.OpcodeEnd} /* invalid because not compiled */},
		},
		parentEngine: e,
	}

	// capturePanic because there's no require.PanicsWithErrorPrefix
	errMessage := capturePanic(func() {
		me.doClose(fakeFinalizer)
	})
	require.Contains(t, errMessage.Error(), "jit: failed to munmap code segment for TestModuleEngine_Close_Panic.function[0]:")
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

func TestModuleEngine_Close_Concurrent(t *testing.T) {
	newFunctionInstance := func(id int) *wasm.FunctionInstance {
		return &wasm.FunctionInstance{
			Name: strconv.Itoa(id), Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}}
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
			e := newEngine()
			var importedModuleEngine *moduleEngine
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
				importedModuleEngine = modEngine.(*moduleEngine)
				require.Len(t, importedModuleEngine.compiledFunctions, len(tc.importedFunctions))
			}

			modEngine, err := e.NewModuleEngine(
				fmt.Sprintf("%s - module-defined functions", t.Name()),
				tc.importedFunctions,
				tc.moduleFunctions,
				nil, // table
				nil, // tableInit
			)
			require.NoError(t, err)
			require.Len(t, modEngine.(*moduleEngine).compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			var importedMappedRegions [][]byte
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
				importedMappedRegions = append(importedMappedRegions, e.compiledFunctions[f].codeSegment)
			}
			var mappedRegions [][]byte
			for _, f := range tc.moduleFunctions {
				require.Contains(t, e.compiledFunctions, f)
				mappedRegions = append(mappedRegions, e.compiledFunctions[f].codeSegment)
			}

			const goroutines = 100
			var wg sync.WaitGroup
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				go func() {
					defer wg.Done()

					// Ensure concurrent multiple execution of Close is guarded by atomic without overloading finalizer.
					modEngine.(*moduleEngine).doClose(fakeFinalizer)
				}()
			}
			wg.Wait()

			require.True(t, modEngine.(*moduleEngine).closed == 1)
			require.Len(t, e.compiledFunctions, len(tc.importedFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for i, f := range tc.moduleFunctions {
				require.NotContains(t, e.compiledFunctions, f, i)
			}

			for _, mappedRegion := range mappedRegions {
				// munmap twice should result in error.
				err = munmapCodeSegment(mappedRegion)
				require.Error(t, err)
			}

			if len(tc.importedFunctions) > 0 {
				importedModuleEngine.doClose(fakeFinalizer)

				for _, mappedRegion := range importedMappedRegions {
					// munmap twice should result in error.
					err = munmapCodeSegment(mappedRegion)
					require.Error(t, err)
				}
			}
		})
	}
}

// Ensures that value stack and call-frame stack are allocated on heap which
// allows us to safely access to their data region from native code.
// See comments on initialValueStackSize and initialCallFrameStackSize.
func TestSliceAllocatedOnHeap(t *testing.T) {
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

		// Trigger relocation of goroutine stack because at this point we have majority of
		// goroutine stack unused after recursive call.
		runtime.GC()
	}})
	require.NoError(t, err)

	_, err = store.Instantiate(context.Background(), hm, hostModuleName)
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

	mi, err := store.Instantiate(context.Background(), m, t.Name())
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
