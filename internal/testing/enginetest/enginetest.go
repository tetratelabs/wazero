// Package enginetest contains tests common to any wasm.Engine implementation. Defining these as top-level
// functions is less burden than copy/pasting the implementations, while still allowing test caching to operate.
//
// In simplest case, dispatch:
//
//	func TestModuleEngine_Call(t *testing.T) {
//		enginetest.RunTestModuleEngineCall(t, NewEngine)
//	}
//
// Some tests using the Compiler Engine may need to guard as they use compiled features:
//
//	func TestModuleEngine_Call(t *testing.T) {
//		requireSupportedOSArch(t)
//		enginetest.RunTestModuleEngineCall(t, NewEngine)
//	}
//
// Note: These tests intentionally avoid using wasm.Store as it is important to know both the dependencies and
// the capabilities at the wasm.Engine abstraction.
//
// TODO: the purpose of enginetest overlaps with the purpose of internal/integration_test/engine. We should
// migrate there since the tests here are costly maintenance-wise.
package enginetest

import (
	"context"
	"debug/dwarf"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

const (
	i32, i64 = wasm.ValueTypeI32, wasm.ValueTypeI64
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

type EngineTester interface {
	NewEngine(enabledFeatures api.CoreFeatures) wasm.Engine

	ListenerFactory() experimental.FunctionListenerFactory
}

func runTestModuleEngineCallHostFnMem(t *testing.T, et EngineTester, readMem *wasm.Code) {
	e := et.NewEngine(api.CoreFeaturesV1)
	defer e.Close()
	importing := setupCallMemTests(t, e, readMem)

	importingMemoryVal := uint64(6)
	importing.MemoryInstance = &wasm.MemoryInstance{Buffer: u64.LeBytes(importingMemoryVal), Min: 1, Cap: 1, Max: 1}

	tests := []struct {
		name     string
		fn       wasm.Index
		expected uint64
	}{
		{
			name:     callImportReadMemName,
			fn:       importing.Exports[callImportReadMemName].Index,
			expected: importingMemoryVal,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ce := importing.Engine.NewFunction(tc.fn)

			results, err := ce.Call(testCtx)
			require.NoError(t, err)
			require.Equal(t, tc.expected, results[0])
		})
	}
}

func RunTestModuleEngineCallHostFn(t *testing.T, et EngineTester) {
	t.Run("wasm", func(t *testing.T) {
		runTestModuleEngineCallHostFn(t, et, hostDivByWasm)
	})
	t.Run("go", func(t *testing.T) {
		runTestModuleEngineCallHostFn(t, et, &hostDivByGo)
		runTestModuleEngineCallHostFnMem(t, et, &hostReadMemGo)
	})
}

func runTestModuleEngineCallHostFn(t *testing.T, et EngineTester, hostDivBy *wasm.Code) {
	e := et.NewEngine(api.CoreFeaturesV1)
	defer e.Close()

	imported, importing := setupCallTests(t, e, hostDivBy, et.ListenerFactory())

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	tests := []struct {
		name   string
		module *wasm.ModuleInstance
		fn     wasm.Index
	}{
		{
			name:   divByWasmName,
			module: imported,
			fn:     imported.Exports[divByWasmName].Index,
		},
		{
			name:   callDivByGoName,
			module: imported,
			fn:     imported.Exports[callDivByGoName].Index,
		},
		{
			name:   callImportCallDivByGoName,
			module: importing,
			fn:     importing.Exports[callImportCallDivByGoName].Index,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.fn

			ce := tc.module.Engine.NewFunction(f)

			results, err := ce.Call(testCtx, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])

			results2, err := ce.Call(testCtx, 1)
			require.NoError(t, err)
			require.Equal(t, results, results2)

			// Ensure the result slices are unique
			results[0] = 255
			require.Equal(t, uint64(1), results2[0])
		})
	}
}

func RunTestModuleEngine_Call_Errors(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV1)
	defer e.Close()

	imported, importing := setupCallTests(t, e, &hostDivByGo, et.ListenerFactory())

	tests := []struct {
		name        string
		module      *wasm.ModuleInstance
		fn          wasm.Index
		input       []uint64
		expectedErr string
	}{
		{
			name:        "wasm function not enough parameters",
			input:       []uint64{},
			module:      imported,
			fn:          imported.Exports[divByWasmName].Index,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "wasm function too many parameters",
			input:       []uint64{1, 2},
			module:      imported,
			fn:          imported.Exports[divByWasmName].Index,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:   "wasm function panics with wasmruntime.Error",
			input:  []uint64{0},
			module: imported,
			fn:     imported.Exports[divByWasmName].Index,
			expectedErr: `wasm error: integer divide by zero
wasm stack trace:
	imported.div_by.wasm(i32) i32`,
		},
		{
			name:   "wasm calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: imported,
			fn:     imported.Exports[callDivByGoName].Index,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing,
			fn:     importing.Exports[callImportCallDivByGoName].Index,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32
	importing.call_import->call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: importing,
			fn:     importing.Exports[callImportCallDivByGoName].Index,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32
	importing.call_import->call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing,
			fn:     importing.Exports[callImportCallDivByGoName].Index,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32
	importing.call_import->call->div_by.go(i32) i32`,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			ce := tc.module.Engine.NewFunction(tc.fn)

			_, err := ce.Call(testCtx, tc.input...)
			require.NotNil(t, err)

			errStr := err.Error()
			// If this faces a Go runtime error, the error includes the Go stack trace which makes the test unstable,
			// so we trim them here.
			if index := strings.Index(errStr, wasmdebug.GoRuntimeErrorTracePrefix); index > -1 {
				errStr = strings.TrimSpace(errStr[:index])
			}
			require.Equal(t, errStr, tc.expectedErr)

			// Ensure the module still works
			results, err := ce.Call(testCtx, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])
		})
	}
}

// RunTestModuleEngineBeforeListenerStackIterator tests that the StackIterator provided by the Engine to the Before hook
// of the listener is properly able to walk the stack.  As an example, it
// validates that the following call stack is properly walked:
//
//  1. f1(2,3,4) [no return, no local]
//  2. calls f2(no arg) [1 return, 1 local]
//  3. calls f3(5) [1 return, no local]
//  4. calls f4(6) [1 return, HOST]
func RunTestModuleEngineBeforeListenerStackIterator(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV2)

	type stackEntry struct {
		debugName string
		args      []uint64
	}

	expectedCallstacks := [][]stackEntry{
		{ // when calling f1
			{debugName: "whatever.f1", args: []uint64{2, 3, 4}},
		},
		{ // when calling f2
			{debugName: "whatever.f2", args: []uint64{}},
			{debugName: "whatever.f1", args: []uint64{2, 3, 4}},
		},
		{ // when calling f3
			{debugName: "whatever.f3", args: []uint64{5}},
			{debugName: "whatever.f2", args: []uint64{}},
			{debugName: "whatever.f1", args: []uint64{2, 3, 4}},
		},
		{ // when calling f4
			{debugName: "whatever.f4", args: []uint64{6}},
			{debugName: "whatever.f3", args: []uint64{5}},
			{debugName: "whatever.f2", args: []uint64{}},
			{debugName: "whatever.f1", args: []uint64{2, 3, 4}},
		},
	}

	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			require.True(t, len(expectedCallstacks) > 0)
			expectedCallstack := expectedCallstacks[0]
			for si.Next() {
				require.True(t, len(expectedCallstack) > 0)
				require.Equal(t, expectedCallstack[0].debugName, si.Function().Definition().DebugName())
				require.Equal(t, expectedCallstack[0].args, si.Parameters())
				expectedCallstack = expectedCallstack[1:]
			}
			require.Equal(t, 0, len(expectedCallstack))
			expectedCallstacks = expectedCallstacks[1:]
		},
	}

	functionTypes := []wasm.FunctionType{
		// f1 type
		{
			Params:            []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			ParamNumInUint64:  3,
			Results:           []api.ValueType{},
			ResultNumInUint64: 0,
		},
		// f2 type
		{
			Params:            []api.ValueType{},
			ParamNumInUint64:  0,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
		// f3 type
		{
			Params:            []api.ValueType{api.ValueTypeI32},
			ParamNumInUint64:  1,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
		// f4 type
		{
			Params:            []api.ValueType{api.ValueTypeI32},
			ParamNumInUint64:  1,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
	}

	hostgofn := wasm.MustParseGoReflectFuncCode(func(x int32) int32 {
		return x + 100
	})

	m := &wasm.Module{
		TypeSection:     functionTypes,
		FunctionSection: []wasm.Index{0, 1, 2, 3},
		NameSection: &wasm.NameSection{
			ModuleName: "whatever",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "f1"},
				{Index: wasm.Index(1), Name: "f2"},
				{Index: wasm.Index(2), Name: "f3"},
				{Index: wasm.Index(3), Name: "f4"},
			},
		},
		CodeSection: []wasm.Code{
			{ // f1
				Body: []byte{
					wasm.OpcodeI32Const, 0, // reserve return for f2
					wasm.OpcodeCall,
					1, // call f2
					wasm.OpcodeEnd,
				},
			},
			{ // f2
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32},
				Body: []byte{
					wasm.OpcodeI32Const, 42, // local for f2
					wasm.OpcodeLocalSet, 0,
					wasm.OpcodeI32Const, 5, // argument of f3
					wasm.OpcodeCall,
					2, // call f3
					wasm.OpcodeEnd,
				},
			},
			{ // f3
				Body: []byte{
					wasm.OpcodeI32Const, 6,
					wasm.OpcodeCall,
					3, // call host function
					wasm.OpcodeEnd,
				},
			},
			// f4 [host function]
			hostgofn,
		},
		ExportSection: []wasm.Export{
			{Name: "f1", Type: wasm.ExternTypeFunc, Index: 0},
		},
		ID: wasm.ModuleID{0},
	}

	listeners := buildFunctionListeners(fnListener, m)
	err := e.CompileModule(testCtx, m, listeners, false)
	require.NoError(t, err)

	module := &wasm.ModuleInstance{
		ModuleName: t.Name(),
		TypeIDs:    []wasm.FunctionTypeID{0, 1, 2, 3},
		Exports:    exportMap(m),
	}

	me, err := e.NewModuleEngine(m, module)
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	initCallEngine := me.NewFunction(0) // f1
	_, err = initCallEngine.Call(testCtx, 2, 3, 4)
	require.NoError(t, err)
	require.Equal(t, 0, len(expectedCallstacks))
}

// This tests that the Globals provided by the Engine to the Before hook of the
// listener is properly able to read the values of the globals.
func RunTestModuleEngineBeforeListenerGlobals(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV2)

	type globals struct {
		values []uint64
		types  []api.ValueType
	}

	expectedGlobals := []globals{
		{values: []uint64{100, 200}, types: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}},
		{values: []uint64{42, 11}, types: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}},
	}

	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			require.True(t, len(expectedGlobals) > 0)

			imod := mod.(experimental.InternalModule)
			expected := expectedGlobals[0]

			require.Equal(t, len(expected.values), imod.NumGlobal())
			for i := 0; i < imod.NumGlobal(); i++ {
				global := imod.Global(i)
				require.Equal(t, expected.types[i], global.Type())
				require.Equal(t, expected.values[i], global.Get())
			}

			expectedGlobals = expectedGlobals[1:]
		},
	}

	functionTypes := []wasm.FunctionType{
		// f1 type
		{
			Params:            []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			ParamNumInUint64:  3,
			Results:           []api.ValueType{},
			ResultNumInUint64: 0,
		},
		// f2 type
		{
			Params:            []api.ValueType{},
			ParamNumInUint64:  0,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
	}

	m := &wasm.Module{
		TypeSection:     functionTypes,
		FunctionSection: []wasm.Index{0, 1},
		NameSection: &wasm.NameSection{
			ModuleName: "whatever",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "f1"},
				{Index: wasm.Index(1), Name: "f2"},
			},
		},
		GlobalSection: []wasm.Global{
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(100)},
			},
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(200)},
			},
		},
		CodeSection: []wasm.Code{
			{ // f1
				Body: []byte{
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeGlobalSet, 0, // store 42 in global 0
					wasm.OpcodeI32Const, 11,
					wasm.OpcodeGlobalSet, 1, // store 11 in global 1
					wasm.OpcodeI32Const, 0, // reserve return for f2
					wasm.OpcodeCall,
					1, // call f2
					wasm.OpcodeEnd,
				},
			},
			{ // f2
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32},
				Body: []byte{
					wasm.OpcodeI32Const, 42, // local for f2
					wasm.OpcodeLocalSet, 0,
					wasm.OpcodeEnd,
				},
			},
		},
		ExportSection: []wasm.Export{
			{Name: "f1", Type: wasm.ExternTypeFunc, Index: 0},
		},
		ID: wasm.ModuleID{0},
	}

	listeners := buildFunctionListeners(fnListener, m)
	err := e.CompileModule(testCtx, m, listeners, false)
	require.NoError(t, err)

	module := &wasm.ModuleInstance{
		ModuleName: t.Name(),
		TypeIDs:    []wasm.FunctionTypeID{0, 1, 2, 3},
		Exports:    exportMap(m),
		Globals: []*wasm.GlobalInstance{
			{Val: 100, Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true}},
			{Val: 200, Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true}},
		},
	}

	me, err := e.NewModuleEngine(m, module)
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	initCallEngine := me.NewFunction(0) // f1
	_, err = initCallEngine.Call(testCtx, 2, 3, 4)
	require.NoError(t, err)
	require.True(t, len(expectedGlobals) == 0)
}

type fnListener struct {
	beforeFn func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator)
	afterFn  func(context.Context, api.Module, api.FunctionDefinition, []uint64)
	abortFn  func(context.Context, api.Module, api.FunctionDefinition, any)
}

func (f *fnListener) NewFunctionListener(api.FunctionDefinition) experimental.FunctionListener {
	return f
}

func (f *fnListener) Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, stackIterator experimental.StackIterator) {
	if f.beforeFn != nil {
		f.beforeFn(ctx, mod, def, params, stackIterator)
	}
}

func (f *fnListener) After(ctx context.Context, mod api.Module, def api.FunctionDefinition, results []uint64) {
	if f.afterFn != nil {
		f.afterFn(ctx, mod, def, results)
	}
}

func (f *fnListener) Abort(ctx context.Context, mod api.Module, def api.FunctionDefinition, err error) {
	if f.abortFn != nil {
		f.abortFn(ctx, mod, def, err)
	}
}

func RunTestModuleEngineStackIteratorOffset(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV2)

	type frame struct {
		function api.FunctionDefinition
		offset   uint64
	}

	var tape [][]frame

	fnListener := &fnListener{
		beforeFn: func(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, si experimental.StackIterator) {
			var stack []frame
			for si.Next() {
				fn := si.Function()
				pc := si.ProgramCounter()
				stack = append(stack, frame{fn.Definition(), fn.SourceOffsetForPC(pc)})
			}
			tape = append(tape, stack)
		},
	}

	functionTypes := []wasm.FunctionType{
		// f1 type
		{
			Params:            []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			ParamNumInUint64:  3,
			Results:           []api.ValueType{},
			ResultNumInUint64: 0,
		},
		// f2 type
		{
			Params:            []api.ValueType{},
			ParamNumInUint64:  0,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
		// f3 type
		{
			Params:            []api.ValueType{api.ValueTypeI32},
			ParamNumInUint64:  1,
			Results:           []api.ValueType{api.ValueTypeI32},
			ResultNumInUint64: 1,
		},
	}

	// Minimal DWARF info section to make debug/dwarf.New() happy.
	// Necessary to make the compiler emit source offset maps.
	info := []byte{
		0x7, 0x0, 0x0, 0x0, // length (len(info) - 4)
		0x3, 0x0, // version (between 3 and 5 makes it easier)
		0x0, 0x0, 0x0, 0x0, // abbrev offset
		0x0, // asize
	}

	d, err := dwarf.New(nil, nil, nil, info, nil, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	hostgofn := wasm.MustParseGoReflectFuncCode(func(x int32) int32 {
		return x + 100
	})

	m := &wasm.Module{
		DWARFLines:      wasmdebug.NewDWARFLines(d),
		TypeSection:     functionTypes,
		FunctionSection: []wasm.Index{0, 1, 2},
		NameSection: &wasm.NameSection{
			ModuleName: "whatever",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(0), Name: "f1"},
				{Index: wasm.Index(1), Name: "f2"},
				{Index: wasm.Index(2), Name: "f3"},
			},
		},
		GlobalSection: []wasm.Global{
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(100)},
			},
			{
				Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(200)},
			},
		},
		CodeSection: []wasm.Code{
			{ // f1
				Body: []byte{
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeGlobalSet, 0, // store 42 in global 0
					wasm.OpcodeI32Const, 11,
					wasm.OpcodeGlobalSet, 1, // store 11 in global 1
					wasm.OpcodeI32Const, 0, // reserve return for f2
					wasm.OpcodeCall, 1, // call f2
					wasm.OpcodeEnd,
				},
			},
			{ // f2
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32},
				Body: []byte{
					wasm.OpcodeI32Const, 42, // local for f2
					wasm.OpcodeLocalSet, 0,
					wasm.OpcodeI32Const, 6,
					wasm.OpcodeCall, 2, // call host function
					wasm.OpcodeEnd,
				},
			},
			// f3
			hostgofn,
		},
		ExportSection: []wasm.Export{
			{Name: "f1", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "f2", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "f3", Type: wasm.ExternTypeFunc, Index: 2},
		},
		ID: wasm.ModuleID{0},
	}

	f1offset := uint64(0)
	f2offset := f1offset + uint64(len(m.CodeSection[0].Body))
	f3offset := f2offset + uint64(len(m.CodeSection[1].Body))
	m.CodeSection[0].BodyOffsetInCodeSection = f1offset
	m.CodeSection[1].BodyOffsetInCodeSection = f2offset

	listeners := buildFunctionListeners(fnListener, m)
	err = e.CompileModule(testCtx, m, listeners, false)
	require.NoError(t, err)

	module := &wasm.ModuleInstance{
		ModuleName: t.Name(),
		TypeIDs:    []wasm.FunctionTypeID{0, 1, 2},
		Exports:    exportMap(m),
		Globals: []*wasm.GlobalInstance{
			{Val: 100, Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true}},
			{Val: 200, Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true}},
		},
		Source: m,
	}

	me, err := e.NewModuleEngine(m, module)
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	initCallEngine := me.NewFunction(0) // f1
	_, err = initCallEngine.Call(testCtx, 2, 3, 4)
	require.NoError(t, err)

	defs := module.ExportedFunctionDefinitions()
	f1 := defs["f1"]
	f2 := defs["f2"]
	f3 := defs["f3"]
	t.Logf("f1 offset: %#x", f1offset)
	t.Logf("f2 offset: %#x", f2offset)
	t.Logf("f3 offset: %#x", f3offset)

	expectedStacks := [][]frame{
		{
			{f1, f1offset + 0},
		},
		{
			{f2, f2offset + 0},
			{f1, f1offset + 10}, // index of call opcode in f1's code
		},
		{
			{f3, 0},             // host functions don't have a wasm code offset
			{f2, f2offset + 6},  // index of call opcode in f2's code
			{f1, f1offset + 10}, // index of call opcode in f1's code
		},
	}

	for si, stack := range tape {
		t.Log("Recorded stack", si, ":")
		require.True(t, len(expectedStacks) > 0, "more recorded stacks than expected stacks")
		expectedStack := expectedStacks[0]
		expectedStacks = expectedStacks[1:]
		for fi, frame := range stack {
			t.Logf("\t%d -> %s :: %#x", fi, frame.function.Name(), frame.offset)
			require.True(t, len(expectedStack) > 0, "more frames in stack than expected")
			expectedFrame := expectedStack[0]
			expectedStack = expectedStack[1:]
			require.Equal(t, expectedFrame, frame)
		}
		require.Zero(t, len(expectedStack), "expected more frames in stack")
	}
	require.Zero(t, len(expectedStacks), "expected more stacks")
}

const (
	divByWasmName             = "div_by.wasm"
	divByGoName               = "div_by.go"
	callDivByGoName           = "call->" + divByGoName
	callImportCallDivByGoName = "call_import->" + callDivByGoName
)

func divByGo(d uint32) uint32 {
	if d == math.MaxUint32 {
		panic(errors.New("host-function panic"))
	}
	return 1 / d // go panics if d == 0
}

var hostDivByGo = wasm.MustParseGoReflectFuncCode(divByGo)

// (func (export "div_by.wasm") (param i32) (result i32) (i32.div_u (i32.const 1) (local.get 0)))
var (
	divByWasm     = []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeLocalGet, 0, wasm.OpcodeI32DivU, wasm.OpcodeEnd}
	hostDivByWasm = &wasm.Code{Body: divByWasm}
)

const (
	readMemName           = "read_mem"
	callImportReadMemName = "call_import->read_mem"
)

func readMemGo(_ context.Context, m api.Module) uint64 {
	ret, ok := m.Memory().ReadUint64Le(0)
	if !ok {
		panic("couldn't read memory")
	}
	return ret
}

var hostReadMemGo = wasm.MustParseGoReflectFuncCode(readMemGo)

func setupCallTests(t *testing.T, e wasm.Engine, divBy *wasm.Code, fnlf experimental.FunctionListenerFactory) (*wasm.ModuleInstance, *wasm.ModuleInstance) {
	ft := wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}, ParamNumInUint64: 1, ResultNumInUint64: 1}

	divByName := divByWasmName
	if divBy.GoFunc != nil {
		divByName = divByGoName
	}
	hostModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []wasm.Code{*divBy},
		ExportSection:   []wasm.Export{{Name: divByGoName, Type: wasm.ExternTypeFunc, Index: 0}},
		NameSection: &wasm.NameSection{
			ModuleName:    "host",
			FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: divByName}},
		},
		ID: wasm.ModuleID{0},
	}
	lns := buildFunctionListeners(fnlf, hostModule)
	err := e.CompileModule(testCtx, hostModule, lns, false)
	require.NoError(t, err)
	host := &wasm.ModuleInstance{
		ModuleName: hostModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0},
		Source: hostModule,
	}
	host.Exports = exportMap(hostModule)

	hostME, err := e.NewModuleEngine(hostModule, host)
	require.NoError(t, err)
	linkModuleToEngine(host, hostME)

	importedModule := &wasm.Module{
		ImportFunctionCount: 1,
		ImportSection:       []wasm.Import{{}},
		TypeSection:         []wasm.FunctionType{ft},
		FunctionSection:     []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			{Body: divByWasm},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, byte(0), // Calling imported host function ^.
				wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{
			{Name: divByWasmName, Type: wasm.ExternTypeFunc, Index: 1},
			{Name: callDivByGoName, Type: wasm.ExternTypeFunc, Index: 2},
		},
		NameSection: &wasm.NameSection{
			ModuleName: "imported",
			FunctionNames: wasm.NameMap{
				{Index: wasm.Index(1), Name: divByWasmName},
				{Index: wasm.Index(2), Name: callDivByGoName},
			},
		},
		ID: wasm.ModuleID{1},
	}
	lns = buildFunctionListeners(fnlf, importedModule)
	err = e.CompileModule(testCtx, importedModule, lns, false)
	require.NoError(t, err)

	imported := &wasm.ModuleInstance{
		Source:     importedModule,
		ModuleName: importedModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0},
	}
	imported.Exports = exportMap(importedModule)

	// Compile the imported module
	importedMe, err := e.NewModuleEngine(importedModule, imported)
	require.NoError(t, err)
	linkModuleToEngine(imported, importedMe)
	importedMe.ResolveImportedFunction(0, 0, hostME)

	// To test stack traces, call the same function from another module
	importingModule := &wasm.Module{
		ImportFunctionCount: 1,
		TypeSection:         []wasm.FunctionType{ft},
		ImportSection:       []wasm.Import{{}},
		FunctionSection:     []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0 /* only one imported function */, wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{
			{Name: callImportCallDivByGoName, Type: wasm.ExternTypeFunc, Index: 1},
		},
		NameSection: &wasm.NameSection{
			ModuleName:    "importing",
			FunctionNames: wasm.NameMap{{Index: wasm.Index(1), Name: callImportCallDivByGoName}},
		},
		ID: wasm.ModuleID{2},
	}
	lns = buildFunctionListeners(fnlf, importingModule)
	err = e.CompileModule(testCtx, importingModule, lns, false)
	require.NoError(t, err)

	// Add the exported function.
	importing := &wasm.ModuleInstance{
		ModuleName: importingModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0},
		Source: importingModule,
	}
	importing.Exports = exportMap(importingModule)

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importingModule, importing)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)
	importingMe.ResolveImportedFunction(0, 2, importedMe)
	return imported, importing
}

func setupCallMemTests(t *testing.T, e wasm.Engine, readMem *wasm.Code) *wasm.ModuleInstance {
	ft := wasm.FunctionType{Results: []wasm.ValueType{i64}, ResultNumInUint64: 1}

	hostModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []wasm.Code{*readMem},
		ExportSection: []wasm.Export{
			{Name: readMemName, Type: wasm.ExternTypeFunc, Index: 0},
		},
		NameSection: &wasm.NameSection{
			ModuleName:    "host",
			FunctionNames: wasm.NameMap{{Index: 0, Name: readMemName}},
		},
		ID: wasm.ModuleID{0},
	}
	err := e.CompileModule(testCtx, hostModule, nil, false)
	require.NoError(t, err)
	host := &wasm.ModuleInstance{ModuleName: hostModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	host.Exports = exportMap(hostModule)

	hostMe, err := e.NewModuleEngine(hostModule, host)
	require.NoError(t, err)
	linkModuleToEngine(host, hostMe)

	importingModule := &wasm.Module{
		ImportFunctionCount: 1,
		TypeSection:         []wasm.FunctionType{ft},
		ImportSection: []wasm.Import{
			// Placeholder for two import functions from `importedModule`.
			{Type: wasm.ExternTypeFunc, DescFunc: 0},
		},
		FunctionSection: []wasm.Index{0},
		ExportSection: []wasm.Export{
			{Name: callImportReadMemName, Type: wasm.ExternTypeFunc, Index: 1},
		},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Calling the index 1 = readMemFn.
		},
		NameSection: &wasm.NameSection{
			ModuleName: "importing",
			FunctionNames: wasm.NameMap{
				{Index: 2, Name: callImportReadMemName},
			},
		},
		// Indicates that this module has a memory so that compilers are able to assembe memory-related initialization.
		MemorySection: &wasm.Memory{Min: 1},
		ID:            wasm.ModuleID{1},
	}
	err = e.CompileModule(testCtx, importingModule, nil, false)
	require.NoError(t, err)

	// Add the exported function.
	importing := &wasm.ModuleInstance{ModuleName: importingModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	// Note: adds imported functions readMemFn and callReadMemFn at index 0 and 1.
	importing.Exports = exportMap(importingModule)

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importingModule, importing)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)
	importingMe.ResolveImportedFunction(0, 0, hostMe)
	return importing
}

// linkModuleToEngine assigns fields that wasm.Store would on instantiation. These include fields both interpreter and
// Compiler needs as well as fields only needed by Compiler.
//
// Note: This sets fields that are not needed in the interpreter, but are required by code compiled by Compiler. If a new
// test here passes in the interpreter and segmentation faults in Compiler, check for a new field offset or a change in Compiler
// (e.g. compiler.TestVerifyOffsetValue). It is possible for all other tests to pass as that field is implicitly set by
// wasm.Store: store isn't used here for unit test precision.
func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me // for Compiler, links the module to the module-engine compiled from it (moduleInstanceEngineOffset).
}

func buildFunctionListeners(factory experimental.FunctionListenerFactory, m *wasm.Module) []experimental.FunctionListener {
	if factory == nil || len(m.FunctionSection) == 0 {
		return nil
	}
	listeners := make([]experimental.FunctionListener, len(m.FunctionSection))
	importCount := m.ImportFunctionCount
	for i := 0; i < len(listeners); i++ {
		listeners[i] = factory.NewFunctionListener(m.FunctionDefinition(uint32(i) + importCount))
	}
	return listeners
}

func exportMap(m *wasm.Module) map[string]*wasm.Export {
	ret := make(map[string]*wasm.Export, len(m.ExportSection))
	for i := range m.ExportSection {
		exp := &m.ExportSection[i]
		ret[exp.Name] = exp
	}
	return ret
}
