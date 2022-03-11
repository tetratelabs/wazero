package jit

import (
	"context"
	"math"
	"reflect"
	"runtime"
	"strconv"
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
	store := wasm.NewStore(context.Background(), e, wasm.Features20191205)
	mod, err := store.Instantiate(m, "")
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
	modCtx := wasm.NewModuleContext(context.Background(), e, module)
	f := &wasm.FunctionInstance{
		GoFunc: &hostFn,
		Kind:   wasm.FunctionKindGoModule,
		Type: &wasm.FunctionType{
			Params:  []wasm.ValueType{wasm.ValueTypeI64},
			Results: []wasm.ValueType{wasm.ValueTypeI64},
		},
		Module: module,
	}

	modEngine, err := e.Compile(nil, []*wasm.FunctionInstance{f})
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
		_, err := e.Compile([]*wasm.FunctionInstance{{Module: &wasm.ModuleInstance{Name: "uncompiled"}, Name: "fn"}}, nil)
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
		_, err := e.Compile(nil, importedFunctions)
		require.NoError(t, err)

		require.Len(t, e.compiledFunctions, len(importedFunctions))

		moduleFunctions := []*wasm.FunctionInstance{
			{Name: "ok1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "ok2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{Name: "invalid code", Type: &wasm.FunctionType{}, Body: []byte{
				wasm.OpcodeCall, // Call instruction without immediate for call target index is invalid and should fail to compile.
			}, Module: &wasm.ModuleInstance{}},
		}

		_, err = e.Compile(importedFunctions, moduleFunctions)
		require.EqualError(t, err, "function[2/2] failed to lower to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failrue, all the compiled functions including suceeded ones must be released.
		require.Len(t, e.compiledFunctions, len(importedFunctions))
		for _, f := range moduleFunctions {
			require.NotContains(t, e.compiledFunctions, f)
		}
	})
}

func TestRelease(t *testing.T) {
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
			if len(tc.importedFunctions) > 0 {
				modEngine, err := e.Compile(nil, tc.importedFunctions)
				require.NoError(t, err)
				require.Len(t, modEngine.(*moduleEngine).compiledFunctions, len(tc.importedFunctions))
			}

			modEngine, err := e.Compile(tc.importedFunctions, tc.moduleFunctions)
			require.NoError(t, err)
			require.Len(t, modEngine.(*moduleEngine).compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions)+len(tc.moduleFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for _, f := range tc.moduleFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}

			err = modEngine.Release()
			require.NoError(t, err)

			require.Len(t, e.compiledFunctions, len(tc.importedFunctions))
			for _, f := range tc.importedFunctions {
				require.Contains(t, e.compiledFunctions, f)
			}
			for i, f := range tc.moduleFunctions {
				require.NotContains(t, e.compiledFunctions, f, i)
			}
		})
	}
}

func TestSliceAllocatedOnHeap(t *testing.T) {
	e := newEngine()
	store := wasm.NewStore(context.Background(), e, wasm.Features20191205)

	hm, err := wasm.NewHostModule("env", map[string]interface{}{
		"grow_stack": func() {
			var growGoroutineStack func(int)
			growGoroutineStack = func(num int) {
				if num == 0 {
					return
				}
				growGoroutineStack(num - 1)
			}
			growGoroutineStack(1000)
			runtime.GC()
		},
	})
	require.NoError(t, err)

	_, err = store.Instantiate(hm, "env")
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
					wasm.OpcodeI32Const, expectedReturnValue,
					wasm.OpcodeEnd,
				},
			},
			{
				// call_stack_corruption
				Body: []byte{
					wasm.OpcodeCall, 3,
					wasm.OpcodeCall, 0,
					wasm.OpcodeI32Const, expectedReturnValue,
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
		},
		ImportSection: []*wasm.Import{{Module: "env", Name: "grow_stack", DescFunc: 1}},
		ExportSection: map[string]*wasm.Export{
			valueStackCorruption: {Type: wasm.ExternTypeFunc, Index: 1, Name: valueStackCorruption},
			callStackCorruption:  {Type: wasm.ExternTypeFunc, Index: 2, Name: callStackCorruption},
		},
	}

	mi, err := store.Instantiate(m, "")
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
