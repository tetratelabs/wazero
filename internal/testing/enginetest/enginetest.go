// Package enginetest contains tests common to any wasm.Engine implementation. Defining these as top-level
// functions is less burden than copy/pasting the implementations, while still allowing test caching to operate.
//
// Ex. In simplest case, dispatch:
//	func TestModuleEngine_Call(t *testing.T) {
//		enginetest.RunTestModuleEngine_Call(t, NewEngine)
//	}
//
// Ex. Some tests using the JIT Engine may need to guard as they use compiled features:
//	func TestModuleEngine_Call(t *testing.T) {
//		requireSupportedOSArch(t)
//		enginetest.RunTestModuleEngine_Call(t, NewEngine)
//	}
//
// Note: These tests intentionally avoid using wasm.Store as it is important to know both the dependencies and
// the capabilities at the wasm.Engine abstraction.
package enginetest

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type EngineTester interface {
	NewEngine() wasm.Engine
	InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{}
}

func RunTestEngine_NewModuleEngine(t *testing.T, et EngineTester) {
	e := et.NewEngine()

	t.Run("sets module name", func(t *testing.T) {
		me, err := e.NewModuleEngine(t.Name(), nil, nil, nil, nil)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, me, 0)
		require.Equal(t, t.Name(), me.Name())
	})
}

func RunTestModuleEngine_Call(t *testing.T, et EngineTester) {
	e := et.NewEngine()

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	i64 := wasm.ValueTypeI64
	fn := &wasm.FunctionInstance{
		Kind: wasm.FunctionKindWasm,
		Type: &wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}},
		Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd},
	}

	// To use the function, we first need to add it to a module.
	module := &wasm.ModuleInstance{Name: t.Name()}
	addFunction(module, "fn", fn)

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, nil, module.Functions, nil, nil)
	require.NoError(t, err)
	defer closeModuleEngineWithExitCode(t, me, 0)
	linkModuleToEngine(module, me)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	results, err := me.Call(module.Ctx, fn, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := me.Call(module.Ctx, fn)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := me.Call(module.Ctx, fn, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func RunTestEngine_NewModuleEngine_InitTable(t *testing.T, et EngineTester) {
	e := et.NewEngine()

	t.Run("no table elements", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		var moduleFunctions []*wasm.FunctionInstance
		var tableInit map[wasm.Index]wasm.Index

		// Instantiate the module, which has nothing but an empty table.
		me, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, me, 0)

		// Since there are no elements to initialize, we expect the table to be nil.
		require.Equal(t, table.Table, make([]interface{}, 2))
	})

	t.Run("module-defined function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		moduleFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		tableInit := map[wasm.Index]wasm.Index{0: 2}

		// Instantiate the module whose table points to its own functions.
		me, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, me, 0)

		// The functions mapped to the table are defined in the same moduleEngine
		require.Equal(t, table.Table, et.InitTable(me, table.Min, tableInit))
	})

	t.Run("imported function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		importedFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		var moduleFunctions []*wasm.FunctionInstance
		tableInit := map[wasm.Index]wasm.Index{0: 2}

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, imported, 0)

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, importing, 0)

		// A moduleEngine's compiled function slice includes its imports, so the offsets is absolute.
		require.Equal(t, table.Table, et.InitTable(importing, table.Min, tableInit))
	})

	t.Run("mixed functions", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		importedFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		moduleFunctions := []*wasm.FunctionInstance{
			{DebugName: "1", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "2", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "3", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
			{DebugName: "4", Type: &wasm.FunctionType{}, Body: []byte{wasm.OpcodeEnd}, Module: &wasm.ModuleInstance{}},
		}
		tableInit := map[wasm.Index]wasm.Index{0: 0, 1: 4}

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, imported, 0)

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer closeModuleEngineWithExitCode(t, importing, 0)

		// A moduleEngine's compiled function slice includes its imports, so the offsets are absolute.
		require.Equal(t, table.Table, et.InitTable(importing, table.Min, tableInit))
	})
}

func runTestModuleEngine_Call_HostFn_ModuleContext(t *testing.T, et EngineTester) {
	memory := &wasm.MemoryInstance{}
	var ctxMemory api.Memory
	hostFn := reflect.ValueOf(func(ctx api.Module, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := et.NewEngine()
	module := &wasm.ModuleInstance{Memory: memory}
	modCtx := wasm.NewModuleContext(context.Background(), wasm.NewStore(e, wasm.Features20191205), module, nil)

	f := &wasm.FunctionInstance{
		GoFunc: &hostFn,
		Kind:   wasm.FunctionKindGoModule,
		Type: &wasm.FunctionType{
			Params:  []wasm.ValueType{wasm.ValueTypeI64},
			Results: []wasm.ValueType{wasm.ValueTypeI64},
		},
		Module: module,
		Index:  0,
	}
	module.Types = []*wasm.TypeInstance{{Type: f.Type}}
	module.Functions = []*wasm.FunctionInstance{f}

	me, err := e.NewModuleEngine(t.Name(), nil, module.Functions, nil, nil)
	require.NoError(t, err)
	defer closeModuleEngineWithExitCode(t, me, 0)

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := me.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})
}

func RunTestModuleEngine_Call_HostFn(t *testing.T, et EngineTester) {
	runTestModuleEngine_Call_HostFn_ModuleContext(t, et) // TODO: refactor to use the same test interface.

	e := et.NewEngine()

	imported, importedMe := setupCallTests(t, e)
	defer closeModuleEngineWithExitCode(t, importedMe, 0)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	tests := []struct {
		name string
		me   wasm.ModuleEngine
		fn   *wasm.FunctionInstance
	}{
		{
			name: wasmFnName,
			me:   importedMe,
			fn:   imported.Exports[wasmFnName].Function,
		},
		{
			name: hostFnName,
			me:   importedMe,
			fn:   imported.Exports[hostFnName].Function,
		},
		{
			name: callHostFnName,
			me:   importedMe,
			fn:   imported.Exports[callHostFnName].Function,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.fn
			m := f.Module
			results, err := m.Engine.Call(m.Ctx, f, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])
		})
	}
}

func RunTestModuleEngine_Call_Errors(t *testing.T, et EngineTester) {
	e := et.NewEngine()

	imported, importedMe := setupCallTests(t, e)
	defer closeModuleEngineWithExitCode(t, importedMe, 0)

	tests := []struct {
		name        string
		me          wasm.ModuleEngine
		fn          *wasm.FunctionInstance
		input       []uint64
		expectedErr string
	}{
		{
			name:        "host function not enough parameters",
			input:       []uint64{},
			me:          importedMe,
			fn:          imported.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "host function too many parameters",
			input:       []uint64{1, 2},
			me:          importedMe,
			fn:          imported.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:        "wasm function not enough parameters",
			input:       []uint64{},
			me:          importedMe,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "wasm function too many parameters",
			input:       []uint64{1, 2},
			me:          importedMe,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:  "wasm function panics with wasmruntime.Error",
			input: []uint64{0},
			me:    importedMe,
			fn:    imported.Exports[wasmFnName].Function,
			expectedErr: `wasm runtime error: integer divide by zero
wasm backtrace:
	0: wasm_div_by`,
		},
		{
			name:  "host function that panics",
			input: []uint64{math.MaxUint32},
			me:    importedMe,
			fn:    imported.Exports[hostFnName].Function,
			expectedErr: `wasm runtime error: host-function panic
wasm backtrace:
	0: host_div_by`,
		},
		{
			name:  "host function panics with runtime.Error",
			input: []uint64{0},
			me:    importedMe,
			fn:    imported.Exports[hostFnName].Function, // TODO: This should be a normal runtime error
			expectedErr: `wasm runtime error: runtime error: integer divide by zero
wasm backtrace:
	0: host_div_by`,
		},
		{
			name:  "wasm calls host function that panics",
			input: []uint64{math.MaxUint32},
			me:    importedMe,
			fn:    imported.Exports[callHostFnName].Function,
			expectedErr: `wasm runtime error: host-function panic
wasm backtrace:
	0: host_div_by
	1: call->host_div_by`,
		},
		{
			name:  "wasm calls imported wasm that calls host function panics with runtime.Error",
			input: []uint64{0},
			me:    importedMe,
			fn:    imported.Exports[callHostFnName].Function,
			expectedErr: `wasm runtime error: runtime error: integer divide by zero
wasm backtrace:
	0: host_div_by
	1: call->host_div_by`,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			f := tc.fn
			m := f.Module
			_, err := m.Engine.Call(m.Ctx, f, tc.input...)
			require.EqualError(t, err, tc.expectedErr)

			// Ensure the module still works
			ret, err := m.Engine.Call(m.Ctx, f, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), ret[0])
		})
	}
}

const (
	wasmFnName     = "wasm_div_by"
	hostFnName     = "host_div_by"
	callHostFnName = "call->" + hostFnName
)

// (func (export "wasm_div_by") (param i32) (result i32) (i32.div_u (i32.const 1) (local.get 0)))
var wasmFnBody = []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeLocalGet, 0, wasm.OpcodeI32DivU, wasm.OpcodeEnd}

func divBy(d uint32) uint32 {
	if d == math.MaxUint32 {
		panic(errors.New("host-function panic"))
	}
	return 1 / d // go panics if d == 0
}

func setupCallTests(t *testing.T, e wasm.Engine) (*wasm.ModuleInstance, wasm.ModuleEngine) {
	i32 := wasm.ValueTypeI32
	ft := &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}
	wasmFn := &wasm.FunctionInstance{
		Kind:  wasm.FunctionKindWasm,
		Type:  ft,
		Body:  wasmFnBody,
		Index: 0,
	}
	hostFnVal := reflect.ValueOf(divBy)
	hostFn := &wasm.FunctionInstance{
		Kind:   wasm.FunctionKindGoNoContext,
		Type:   ft,
		GoFunc: &hostFnVal,
		Index:  1,
	}
	callHostFn := &wasm.FunctionInstance{
		Kind:  wasm.FunctionKindWasm,
		Type:  ft,
		Body:  []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, byte(hostFn.Index), wasm.OpcodeEnd},
		Index: 2,
	}

	// To use the function, we first need to add it to a module.
	imported := &wasm.ModuleInstance{Name: t.Name()}
	addFunction(imported, wasmFnName, wasmFn)
	addFunction(imported, hostFnName, hostFn)
	addFunction(imported, callHostFnName, callHostFn)

	// Compile the imported module
	importedMe, err := e.NewModuleEngine(imported.Name, nil, imported.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(imported, importedMe)

	return imported, importedMe
}

// linkModuleToEngine assigns fields that wasm.Store would on instantiation. These includes fields both interpreter and
// JIT needs as well as fields only needed by JIT.
//
// Note: This sets fields that are not needed in the interpreter, but are required by code compiled by JIT. If a new
// test here passes in the interpreter and segmentation faults in JIT, check for a new field offset or a change in JIT
// (ex. jit.TestVerifyOffsetValue). It is possible for all other tests to pass as that field is implicitly set by
// wasm.Store: store isn't used here for unit test precision.
func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me // for JIT, links the module to the module-engine compiled from it (moduleInstanceEngineOffset).
	// callEngineModuleContextModuleInstanceAddressOffset
	module.Ctx = wasm.NewModuleContext(context.Background(), nil, module, nil)
}

// addFunction assigns and adds a function to the module.
func addFunction(module *wasm.ModuleInstance, funcName string, fn *wasm.FunctionInstance) {
	fn.DebugName = funcName
	module.Functions = append(module.Functions, fn)
	if module.Exports == nil {
		module.Exports = map[string]*wasm.ExportInstance{}
	}
	module.Exports[funcName] = &wasm.ExportInstance{Type: wasm.ExternTypeFunc, Function: fn}
	// This link is essential for all engines. For example, functions call other functions defined in the same module.
	fn.Module = module
}

// closeModuleEngineWithExitCode allows unit tests to check `CloseWithExitCode` didn't err.
func closeModuleEngineWithExitCode(t *testing.T, me wasm.ModuleEngine, exitCode uint32) bool {
	ok, err := me.CloseWithExitCode(exitCode)
	require.NoError(t, err)
	return ok
}
