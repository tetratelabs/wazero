package compiler

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/enginetest"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// et is used for tests defined in the enginetest package.
var et = &engineTester{}

// engineTester implements enginetest.EngineTester.
type engineTester struct{}

// IsCompiler implements the same method as documented on enginetest.EngineTester.
func (e *engineTester) IsCompiler() bool {
	return true
}

// ListenerFactory implements the same method as documented on enginetest.EngineTester.
func (e *engineTester) ListenerFactory() experimental.FunctionListenerFactory {
	return nil
}

// NewEngine implements the same method as documented on enginetest.EngineTester.
func (e *engineTester) NewEngine(enabledFeatures api.CoreFeatures) wasm.Engine {
	return newEngine(context.Background(), enabledFeatures)
}

// InitTables implements the same method as documented on enginetest.EngineTester.
func (e engineTester) InitTables(me wasm.ModuleEngine, tableIndexToLen map[wasm.Index]int, tableInits []wasm.TableInitEntry) [][]wasm.Reference {
	references := make([][]wasm.Reference, len(tableIndexToLen))
	for tableIndex, l := range tableIndexToLen {
		references[tableIndex] = make([]uintptr, l)
	}
	internal := me.(*moduleEngine)

	for _, init := range tableInits {
		referencesPerTable := references[init.TableIndex]
		for idx, fnidx := range init.FunctionIndexes {
			referencesPerTable[int(init.Offset)+idx] = uintptr(unsafe.Pointer(internal.functions[*fnidx]))
		}
	}
	return references
}

// CompiledFunctionPointerValue implements the same method as documented on enginetest.EngineTester.
func (e engineTester) CompiledFunctionPointerValue(me wasm.ModuleEngine, funcIndex wasm.Index) uint64 {
	internal := me.(*moduleEngine)
	return uint64(uintptr(unsafe.Pointer(internal.functions[funcIndex])))
}

func TestCompiler_Engine_NewModuleEngine(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestEngine_NewModuleEngine(t, et)
}

func TestCompiler_Engine_InitializeFuncrefGlobals(t *testing.T) {
	enginetest.RunTestEngine_InitializeFuncrefGlobals(t, et)
}

func TestCompiler_Engine_NewModuleEngine_InitTable(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestEngine_NewModuleEngine_InitTable(t, et)
}

func TestCompiler_ModuleEngine_Call(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call(t, et)
}

func TestCompiler_ModuleEngine_Call_HostFn(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call_HostFn(t, et)
}

func TestCompiler_ModuleEngine_Call_Errors(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Call_Errors(t, et)
}

func TestCompiler_ModuleEngine_Memory(t *testing.T) {
	requireSupportedOSArch(t)
	enginetest.RunTestModuleEngine_Memory(t, et)
}

// requireSupportedOSArch is duplicated also in the platform package to ensure no cyclic dependency.
func requireSupportedOSArch(t *testing.T) {
	if !platform.CompilerSupported() {
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

func TestCompiler_CompileModule(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		e := et.NewEngine(api.CoreFeaturesV1).(*engine)
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
		errModule.BuildFunctionDefinitions()

		e := et.NewEngine(api.CoreFeaturesV1).(*engine)
		err := e.CompileModule(testCtx, errModule)
		require.EqualError(t, err, "failed to lower func[.$2] to wazeroir: handling instruction: apply stack failed for call: reading immediates: EOF")

		// On the compilation failure, the compiled functions must not be cached.
		_, ok := e.codes[errModule.ID]
		require.False(t, ok)
	})
}

// TestCompiler_Releasecode_Panic tests that an unexpected panic has some identifying information in it.
func TestCompiler_Releasecode_Panic(t *testing.T) {
	captured := require.CapturePanic(func() {
		releaseCode(&code{
			indexInModule: 2,
			sourceModule:  &wasm.Module{NameSection: &wasm.NameSection{ModuleName: t.Name()}},
			codeSegment:   []byte{wasm.OpcodeEnd}, // never compiled means it was never mapped.
		})
	})
	require.Contains(t, captured.Error(), fmt.Sprintf("compiler: failed to munmap code segment for %[1]s.function[2]", t.Name()))
}

// Ensures that value stack and call-frame stack are allocated on heap which
// allows us to safely access to their data region from native code.
// See comments on initialStackSize and initialCallFrameStackSize.
func TestCompiler_SliceAllocatedOnHeap(t *testing.T) {
	enabledFeatures := api.CoreFeaturesV1
	e := newEngine(context.Background(), enabledFeatures)
	s, ns := wasm.NewStore(enabledFeatures, e)

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
	}}, nil, enabledFeatures)
	require.NoError(t, err)

	err = s.Engine.CompileModule(testCtx, hm)
	require.NoError(t, err)

	_, err = s.Instantiate(testCtx, ns, hm, hostModuleName, nil, nil)
	require.NoError(t, err)

	const stackCorruption = "value_stack_corruption"
	const callStackCorruption = "call_stack_corruption"
	const expectedReturnValue = 0x1
	m := &wasm.Module{
		TypeSection: []*wasm.FunctionType{
			{Params: []wasm.ValueType{}, Results: []wasm.ValueType{wasm.ValueTypeI32}, ResultNumInUint64: 1},
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
			{Type: wasm.ExternTypeFunc, Index: 1, Name: stackCorruption},
			{Type: wasm.ExternTypeFunc, Index: 2, Name: callStackCorruption},
		},
		ID: wasm.ModuleID{1},
	}
	m.BuildFunctionDefinitions()

	err = s.Engine.CompileModule(testCtx, m)
	require.NoError(t, err)

	mi, err := s.Instantiate(testCtx, ns, m, t.Name(), nil, nil)
	require.NoError(t, err)

	for _, fnName := range []string{stackCorruption, callStackCorruption} {
		fnName := fnName
		t.Run(fnName, func(t *testing.T) {
			ret, err := mi.ExportedFunction(fnName).Call(testCtx)
			require.NoError(t, err)

			require.Equal(t, uint32(expectedReturnValue), uint32(ret[0]))
		})
	}
}

func TestCallEngine_builtinFunctionTableGrow(t *testing.T) {
	ce := &callEngine{
		stack: []uint64{
			0xff, // pseudo-ref
			1,    // num
			// Table Index = 0 (lower 32-bits), but the higher bits (32-63) are all sets,
			// which happens if the previous value on that stack location was 64-bit wide.
			0xffffffff << 32,
		},
		stackContext: stackContext{stackPointer: 3},
	}

	table := &wasm.TableInstance{References: []wasm.Reference{}, Min: 10}
	ce.builtinFunctionTableGrow(context.Background(), []*wasm.TableInstance{table})

	require.Equal(t, 1, len(table.References))
	require.Equal(t, uintptr(0xff), table.References[0])
}

func ptrAsUint64(f *function) uint64 {
	return uint64(uintptr(unsafe.Pointer(f)))
}

func TestCallEngine_deferredOnCall(t *testing.T) {
	f1 := &function{source: &wasm.FunctionInstance{
		FunctionDefinition: newMockFunctionDefinition("1"),
		Type:               &wasm.FunctionType{ParamNumInUint64: 2},
	}}
	f2 := &function{source: &wasm.FunctionInstance{
		FunctionDefinition: newMockFunctionDefinition("2"),
		Type:               &wasm.FunctionType{ParamNumInUint64: 2, ResultNumInUint64: 3},
	}}
	f3 := &function{source: &wasm.FunctionInstance{
		FunctionDefinition: newMockFunctionDefinition("3"),
		Type:               &wasm.FunctionType{ResultNumInUint64: 1},
	}}

	ce := &callEngine{
		stack: []uint64{
			0xff, 0xff, // dummy argument for f1
			0, 0, 0, 0,
			0xcc, 0xcc, // local variable for f1.
			// <----- stack base point of f2 (top) == index 8.
			0xaa, 0xaa, 0xdeadbeaf, // dummy argument for f2 (0xaa, 0xaa) and the reserved slot for result 0xdeadbeaf)
			0, 0, ptrAsUint64(f1), 0, // callFrame
			0xcc, 0xcc, 0xcc, // local variable for f2.
			// <----- stack base point of f3 (top) == index 18
			0xdeadbeaf,                    // the reserved slot for result 0xdeadbeaf) from f3.
			0, 8 << 3, ptrAsUint64(f2), 0, // callFrame
		},
		stackContext: stackContext{
			stackBasePointerInBytes: 18 << 3, // currently executed function (f3)'s base pointer.
			stackPointer:            0xff,    // dummy supposed to be reset to zero.
		},
		moduleContext: moduleContext{
			fn:                    f3, // currently executed function (f3)!
			moduleInstanceAddress: 0xdeafbeaf,
		},
	}

	beforeRecoverStack := ce.stack

	err := ce.deferredOnCall(errors.New("some error"))
	require.EqualError(t, err, `some error (recovered by wazero)
wasm stack trace:
	3()
	2()
	1()`)

	// After recover, the state of callEngine must be reset except that the underlying slices must be intact
	// for the subsequent calls to avoid additional allocations on each call.
	require.Equal(t, uint64(0), ce.stackBasePointerInBytes)
	require.Equal(t, uint64(0), ce.stackPointer)
	require.Equal(t, uintptr(0), ce.moduleInstanceAddress)
	require.Equal(t, beforeRecoverStack, ce.stack)

	// Keep f1, f2, and f3 alive until we reach here, as we access these functions from the uint64 raw pointers in the stack.
	// In practice, they are guaranteed to be alive as they are held by moduleContext.
	runtime.KeepAlive(f1)
	runtime.KeepAlive(f2)
	runtime.KeepAlive(f3)
}

func newMockFunctionDefinition(name string) api.FunctionDefinition {
	return &mockFunctionDefinition{debugName: name, FunctionDefinition: &wasm.FunctionDefinition{}}
}

type mockFunctionDefinition struct {
	debugName string
	*wasm.FunctionDefinition
}

// DebugName implements the same method as documented on api.FunctionDefinition.
func (f *mockFunctionDefinition) DebugName() string {
	return f.debugName
}

// ParamTypes implements api.FunctionDefinition ParamTypes.
func (f *mockFunctionDefinition) ParamTypes() []wasm.ValueType {
	return []wasm.ValueType{}
}

// ResultTypes implements api.FunctionDefinition ResultTypes.
func (f *mockFunctionDefinition) ResultTypes() []wasm.ValueType {
	return []wasm.ValueType{}
}

func TestCallEngine_initializeStack(t *testing.T) {
	const i32 = wasm.ValueTypeI32
	const stackSize = 10
	const initialVal = ^uint64(0)
	tests := []struct {
		name            string
		funcType        *wasm.FunctionType
		args            []uint64
		expStackPointer uint64
		expStack        [stackSize]uint64
	}{
		{
			name:            "no param/result",
			funcType:        &wasm.FunctionType{},
			expStackPointer: callFrameDataSizeInUint64,
			expStack: [stackSize]uint64{
				0, 0, 0, // zeroed call frame
				initialVal, initialVal, initialVal, initialVal, initialVal, initialVal, initialVal,
			},
		},
		{
			name: "no result",
			funcType: &wasm.FunctionType{
				Params:           []wasm.ValueType{i32, i32},
				ParamNumInUint64: 2,
			},
			args:            []uint64{0xdeadbeaf, 0xdeadbeaf},
			expStackPointer: callFrameDataSizeInUint64 + 2,
			expStack: [stackSize]uint64{
				0xdeadbeaf, 0xdeadbeaf, // arguments
				0, 0, 0, // zeroed call frame
				initialVal, initialVal, initialVal, initialVal, initialVal,
			},
		},
		{
			name: "no param",
			funcType: &wasm.FunctionType{
				Results:           []wasm.ValueType{i32, i32, i32},
				ResultNumInUint64: 3,
			},
			expStackPointer: callFrameDataSizeInUint64 + 3,
			expStack: [stackSize]uint64{
				initialVal, initialVal, initialVal, // reserved slots for results
				0, 0, 0, // zeroed call frame
				initialVal, initialVal, initialVal, initialVal,
			},
		},
		{
			name: "params > results",
			funcType: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, i32, i32, i32, i32},
				ParamNumInUint64:  5,
				Results:           []wasm.ValueType{i32, i32, i32},
				ResultNumInUint64: 3,
			},
			args:            []uint64{0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf},
			expStackPointer: callFrameDataSizeInUint64 + 5,
			expStack: [stackSize]uint64{
				0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf,
				0, 0, 0, // zeroed call frame
				initialVal, initialVal,
			},
		},
		{
			name: "params == results",
			funcType: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, i32, i32, i32, i32},
				ParamNumInUint64:  5,
				Results:           []wasm.ValueType{i32, i32, i32, i32, i32},
				ResultNumInUint64: 5,
			},
			args:            []uint64{0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf},
			expStackPointer: callFrameDataSizeInUint64 + 5,
			expStack: [stackSize]uint64{
				0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf,
				0, 0, 0, // zeroed call frame
				initialVal, initialVal,
			},
		},
		{
			name: "params < results",
			funcType: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, i32, i32},
				ParamNumInUint64:  3,
				Results:           []wasm.ValueType{i32, i32, i32, i32, i32},
				ResultNumInUint64: 5,
			},
			args:            []uint64{0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf},
			expStackPointer: callFrameDataSizeInUint64 + 5,
			expStack: [stackSize]uint64{
				0xdeafbeaf, 0xdeafbeaf, 0xdeafbeaf,
				initialVal, initialVal, // reserved for results
				0, 0, 0, // zeroed call frame
				initialVal, initialVal,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			initialStack := make([]uint64, stackSize)
			for i := range initialStack {
				initialStack[i] = initialVal
			}
			ce := &callEngine{stack: initialStack}
			ce.initializeStack(tc.funcType, tc.args)
			require.Equal(t, tc.expStackPointer, ce.stackPointer)
			require.Equal(t, tc.expStack[:], ce.stack)
		})
	}
}

func Test_callFrameOffset(t *testing.T) {
	require.Equal(t, 1, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 0, ResultNumInUint64: 1}))
	require.Equal(t, 10, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 5, ResultNumInUint64: 10}))
	require.Equal(t, 100, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 50, ResultNumInUint64: 100}))
	require.Equal(t, 1, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 1, ResultNumInUint64: 0}))
	require.Equal(t, 10, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 10, ResultNumInUint64: 5}))
	require.Equal(t, 100, callFrameOffset(&wasm.FunctionType{ParamNumInUint64: 100, ResultNumInUint64: 50}))
}
