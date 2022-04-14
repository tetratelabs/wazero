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

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

type EngineTester interface {
	NewEngine(enabledFeatures wasm.Features) wasm.Engine
	InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{}
}

func RunTestEngine_NewModuleEngine(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	t.Run("sets module name", func(t *testing.T) {
		me, err := e.NewModuleEngine(t.Name(), nil, nil, nil, nil, nil)
		require.NoError(t, err)
		defer me.Close()
		require.Equal(t, t.Name(), me.Name())
	})
}

func RunTestModuleEngine_Call(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

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
	me, err := e.NewModuleEngine(module.Name, nil, nil, module.Functions, nil, nil)
	require.NoError(t, err)
	defer me.Close()
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
	e := et.NewEngine(wasm.Features20191205)

	t.Run("no table elements", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		var moduleFunctions []*wasm.FunctionInstance
		var tableInit map[wasm.Index]wasm.Index

		// Instantiate the module, which has nothing but an empty table.
		me, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer me.Close()

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
		me, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer me.Close()

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
		imported, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		defer imported.Close()

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer importing.Close()

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
		imported, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		defer imported.Close()

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer importing.Close()

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

	features := wasm.Features20191205
	e := et.NewEngine(features)
	module := &wasm.ModuleInstance{Memory: memory}
	modCtx := wasm.NewModuleContext(context.Background(), wasm.NewStore(features, e), module, nil)

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

	me, err := e.NewModuleEngine(t.Name(), &wasm.Module{}, nil, module.Functions, nil, nil)
	require.NoError(t, err)
	defer me.Close()

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

	e := et.NewEngine(wasm.Features20191205)

	imported, importedMe, importing, importingMe := setupCallTests(t, e)
	defer importingMe.Close()
	defer importedMe.Close()

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	tests := []struct {
		name   string
		module *wasm.ModuleContext
		fn     *wasm.FunctionInstance
	}{
		{
			name:   wasmFnName,
			module: imported.Ctx,
			fn:     imported.Exports[wasmFnName].Function,
		},
		{
			name:   hostFnName,
			module: imported.Ctx,
			fn:     imported.Exports[hostFnName].Function,
		},
		{
			name:   callHostFnName,
			module: imported.Ctx,
			fn:     imported.Exports[callHostFnName].Function,
		},
		{
			name:   callImportCallHostFnName,
			module: importing.Ctx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := tc.module
			f := tc.fn
			results, err := f.Module.Engine.Call(m, f, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])
		})
	}
}

func RunTestModuleEngine_Call_Errors(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	imported, importedMe, importing, importingMe := setupCallTests(t, e)
	defer importingMe.Close()
	defer importedMe.Close()

	tests := []struct {
		name        string
		module      *wasm.ModuleContext
		fn          *wasm.FunctionInstance
		input       []uint64
		expectedErr string
	}{
		{
			name:        "host function not enough parameters",
			input:       []uint64{},
			module:      imported.Ctx,
			fn:          imported.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "host function too many parameters",
			input:       []uint64{1, 2},
			module:      imported.Ctx,
			fn:          imported.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:        "wasm function not enough parameters",
			input:       []uint64{},
			module:      imported.Ctx,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "wasm function too many parameters",
			input:       []uint64{1, 2},
			module:      imported.Ctx,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:   "wasm function panics with wasmruntime.Error",
			input:  []uint64{0},
			module: imported.Ctx,
			fn:     imported.Exports[wasmFnName].Function,
			expectedErr: `wasm error: integer divide by zero
wasm stack trace:
	imported.wasm_div_by(i32) i32`,
		},
		{
			name:   "host function that panics",
			input:  []uint64{math.MaxUint32},
			module: imported.Ctx,
			fn:     imported.Exports[hostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32`,
		},
		{
			name:   "host function panics with runtime.Error",
			input:  []uint64{0},
			module: imported.Ctx,
			fn:     imported.Exports[hostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: imported.Ctx,
			fn:     imported.Exports[callHostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.Ctx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: importing.Ctx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.Ctx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	imported.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := tc.module
			f := tc.fn
			_, err := f.Module.Engine.Call(m, f, tc.input...)
			require.EqualError(t, err, tc.expectedErr)

			// Ensure the module still works
			results, err := f.Module.Engine.Call(m, f, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])
		})
	}
}

const (
	wasmFnName               = "wasm_div_by"
	hostFnName               = "host_div_by"
	callHostFnName           = "call->" + hostFnName
	callImportCallHostFnName = "call_import->" + callHostFnName
)

// (func (export "wasm_div_by") (param i32) (result i32) (i32.div_u (i32.const 1) (local.get 0)))
var wasmFnBody = []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeLocalGet, 0, wasm.OpcodeI32DivU, wasm.OpcodeEnd}

func divBy(d uint32) uint32 {
	if d == math.MaxUint32 {
		panic(errors.New("host-function panic"))
	}
	return 1 / d // go panics if d == 0
}

func setupCallTests(t *testing.T, e wasm.Engine) (*wasm.ModuleInstance, wasm.ModuleEngine, *wasm.ModuleInstance, wasm.ModuleEngine) {
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
	imported := &wasm.ModuleInstance{Name: "imported"}
	addFunction(imported, wasmFnName, wasmFn)
	addFunction(imported, hostFnName, hostFn)
	addFunction(imported, callHostFnName, callHostFn)

	// Compile the imported module
	importedMe, err := e.NewModuleEngine(imported.Name, &wasm.Module{}, nil, imported.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(imported, importedMe)

	// To test stack traces, call the same function from another module
	importing := &wasm.ModuleInstance{Name: "importing"}

	// Don't add imported functions yet as NewModuleEngine requires them split.
	importedFunctions := []*wasm.FunctionInstance{callHostFn}

	// Add the exported function.
	callImportedHostFn := &wasm.FunctionInstance{
		Kind:  wasm.FunctionKindWasm,
		Type:  ft,
		Body:  []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0 /* only one imported function */, wasm.OpcodeEnd},
		Index: 1, // after import
	}
	addFunction(importing, callImportCallHostFnName, callImportedHostFn)

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importing.Name, &wasm.Module{}, importedFunctions, importing.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)

	// Add the imported functions back to the importing module.
	importing.Functions = append(importedFunctions, importing.Functions...)

	return imported, importedMe, importing, importingMe
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
	fn.DebugName = wasmdebug.FuncName(module.Name, funcName, fn.Index)
	module.Functions = append(module.Functions, fn)
	if module.Exports == nil {
		module.Exports = map[string]*wasm.ExportInstance{}
	}
	module.Exports[funcName] = &wasm.ExportInstance{Type: wasm.ExternTypeFunc, Function: fn}
	// This link is essential for all engines. For example, functions call other functions defined in the same module.
	fn.Module = module
}
