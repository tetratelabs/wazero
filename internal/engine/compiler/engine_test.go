package compiler

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"unsafe"

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
func (e *engineTester) NewEngine(enabledFeatures wasm.Features) wasm.Engine {
	return newEngine(enabledFeatures)
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
		errModule.BuildFunctionDefinitions()

		e := et.NewEngine(wasm.Features20191205).(*engine)
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
// See comments on initialValueStackSize and initialCallFrameStackSize.
func TestCompiler_SliceAllocatedOnHeap(t *testing.T) {
	enabledFeatures := wasm.Features20191205
	e := newEngine(enabledFeatures)
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
	}}, nil, map[string]*wasm.Memory{}, map[string]*wasm.Global{}, enabledFeatures)
	require.NoError(t, err)

	err = s.Engine.CompileModule(testCtx, hm)
	require.NoError(t, err)

	_, err = s.Instantiate(testCtx, ns, hm, hostModuleName, nil, nil)
	require.NoError(t, err)

	const valueStackCorruption = "value_stack_corruption"
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
			{Type: wasm.ExternTypeFunc, Index: 1, Name: valueStackCorruption},
			{Type: wasm.ExternTypeFunc, Index: 2, Name: callStackCorruption},
		},
		ID: wasm.ModuleID{1},
	}
	m.BuildFunctionDefinitions()

	err = s.Engine.CompileModule(testCtx, m)
	require.NoError(t, err)

	mi, err := s.Instantiate(testCtx, ns, m, t.Name(), nil, nil)
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

// TODO: move most of this logic to enginetest.go so that there is less drift between interpreter and compiler
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
