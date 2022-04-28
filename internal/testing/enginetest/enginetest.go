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

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

type EngineTester interface {
	NewEngine(enabledFeatures wasm.Features) wasm.Engine
	InitTable(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{}
}

func RunTestEngine_NewModuleEngine(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	t.Run("error before instantiation", func(t *testing.T) {
		_, err := e.NewModuleEngine("mymod", &wasm.Module{}, nil, nil, nil, nil)
		require.EqualError(t, err, "source module for mymod must be compiled before instantiation")
	})

	t.Run("sets module name", func(t *testing.T) {
		m := &wasm.Module{}
		err := e.CompileModule(testCtx, m)
		require.NoError(t, err)
		me, err := e.NewModuleEngine(t.Name(), m, nil, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, t.Name(), me.Name())
	})
}

func getFunctionInstance(module *wasm.Module, index wasm.Index, moduleInstance *wasm.ModuleInstance) *wasm.FunctionInstance {
	c := module.ImportFuncCount()
	typeIndex := module.FunctionSection[index]
	return &wasm.FunctionInstance{
		Kind:       wasm.FunctionKindWasm,
		Module:     moduleInstance,
		Type:       module.TypeSection[typeIndex],
		Body:       module.CodeSection[index].Body,
		LocalTypes: module.CodeSection[index].LocalTypes,
		Index:      index + c,
	}
}

func RunTestModuleEngine_Call(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []uint32{0},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}, LocalTypes: []wasm.ValueType{wasm.ValueTypeI64}}},
	}

	err := e.CompileModule(testCtx, m)
	require.NoError(t, err)

	// To use the function, we first need to add it to a module.
	module := &wasm.ModuleInstance{Name: t.Name()}
	fn := getFunctionInstance(m, 0, module)
	addFunction(module, "fn", fn)

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, m, nil, module.Functions, nil, nil)
	fn.Module.Engine = me
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	results, err := me.Call(testCtx, module.CallCtx, fn, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(3), results[0])

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := me.Call(testCtx, module.CallCtx, fn)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := me.Call(testCtx, module.CallCtx, fn, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

func RunTestEngine_NewModuleEngine_InitTable(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	t.Run("no table elements", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, References: make([]wasm.Reference, 2)}
		m := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{},
			FunctionSection: []uint32{},
			CodeSection:     []*wasm.Code{},
			ID:              wasm.ModuleID{0},
		}
		err := e.CompileModule(testCtx, m)
		require.NoError(t, err)

		// Instantiate the module, which has nothing but an empty table.
		_, err = e.NewModuleEngine(t.Name(), m, nil, nil, table, nil)
		require.NoError(t, err)

		// Since there are no elements to initialize, we expect the table to be nil.
		require.Equal(t, table.References, make([]wasm.Reference, 2))
	})
	t.Run("module-defined function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, References: make([]wasm.Reference, 2)}

		m := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []uint32{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{1},
		}

		err := e.CompileModule(testCtx, m)
		require.NoError(t, err)

		moduleFunctions := []*wasm.FunctionInstance{
			getFunctionInstance(m, 0, nil),
			getFunctionInstance(m, 1, nil),
			getFunctionInstance(m, 2, nil),
			getFunctionInstance(m, 3, nil),
		}
		tableInit := map[wasm.Index]wasm.Index{0: 2}

		// Instantiate the module whose table points to its own functions.
		me, err := e.NewModuleEngine(t.Name(), m, nil, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// The functions mapped to the table are defined in the same moduleEngine
		require.Equal(t, table.References, et.InitTable(me, table.Min, tableInit))
	})

	t.Run("imported function", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, References: make([]wasm.Reference, 2)}

		importedModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []uint32{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{2},
		}

		err := e.CompileModule(testCtx, importedModule)
		require.NoError(t, err)

		importedModuleInstance := &wasm.ModuleInstance{}
		importedFunctions := []*wasm.FunctionInstance{
			getFunctionInstance(importedModule, 0, importedModuleInstance),
			getFunctionInstance(importedModule, 1, importedModuleInstance),
			getFunctionInstance(importedModule, 2, importedModuleInstance),
			getFunctionInstance(importedModule, 3, importedModuleInstance),
		}
		var moduleFunctions []*wasm.FunctionInstance

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), importedModule, nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		importedModuleInstance.Engine = imported

		// Instantiate the importing module, which is whose table is initialized.
		importingModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{},
			FunctionSection: []uint32{},
			CodeSection:     []*wasm.Code{},
			ID:              wasm.ModuleID{3},
		}
		err = e.CompileModule(testCtx, importingModule)
		require.NoError(t, err)

		tableInit := map[wasm.Index]wasm.Index{0: 2}
		importing, err := e.NewModuleEngine(t.Name(), importingModule, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets is absolute.
		require.Equal(t, table.References, et.InitTable(importing, table.Min, tableInit))
	})

	t.Run("mixed functions", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, References: make([]wasm.Reference, 2)}

		importedModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []uint32{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{4},
		}

		err := e.CompileModule(testCtx, importedModule)
		require.NoError(t, err)
		importedModuleInstance := &wasm.ModuleInstance{}
		importedFunctions := []*wasm.FunctionInstance{
			getFunctionInstance(importedModule, 0, importedModuleInstance),
			getFunctionInstance(importedModule, 1, importedModuleInstance),
			getFunctionInstance(importedModule, 2, importedModuleInstance),
			getFunctionInstance(importedModule, 3, importedModuleInstance),
		}

		// Imported functions are compiled before the importing module is instantiated.
		imported, err := e.NewModuleEngine(t.Name(), importedModule, nil, importedFunctions, nil, nil)
		require.NoError(t, err)
		importedModuleInstance.Engine = imported

		importingModule := &wasm.Module{
			TypeSection:     []*wasm.FunctionType{{}},
			FunctionSection: []uint32{0, 0, 0, 0},
			CodeSection: []*wasm.Code{
				{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
			},
			ID: wasm.ModuleID{5},
		}

		err = e.CompileModule(testCtx, importingModule)
		require.NoError(t, err)

		importingModuleInstance := &wasm.ModuleInstance{}
		moduleFunctions := []*wasm.FunctionInstance{
			getFunctionInstance(importedModule, 0, importingModuleInstance),
			getFunctionInstance(importedModule, 1, importingModuleInstance),
			getFunctionInstance(importedModule, 2, importingModuleInstance),
			getFunctionInstance(importedModule, 3, importingModuleInstance),
		}
		tableInit := map[wasm.Index]wasm.Index{0: 0, 1: 4}

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importingModule, importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets are absolute.
		require.Equal(t, table.References, et.InitTable(importing, table.Min, tableInit))
	})
}

func runTestModuleEngine_Call_HostFn_ModuleContext(t *testing.T, et EngineTester) {
	features := wasm.Features20191205
	e := et.NewEngine(features)

	sig := &wasm.FunctionType{
		Params:  []wasm.ValueType{wasm.ValueTypeI64},
		Results: []wasm.ValueType{wasm.ValueTypeI64},
	}

	memory := &wasm.MemoryInstance{}
	var mMemory api.Memory
	hostFn := reflect.ValueOf(func(m api.Module, v uint64) uint64 {
		mMemory = m.Memory()
		return v
	})

	m := &wasm.Module{
		HostFunctionSection: []*reflect.Value{&hostFn},
		FunctionSection:     []wasm.Index{0},
		TypeSection:         []*wasm.FunctionType{sig},
	}

	err := e.CompileModule(testCtx, m)
	require.NoError(t, err)

	module := &wasm.ModuleInstance{Memory: memory}
	modCtx := wasm.NewCallContext(wasm.NewStore(features, e), module, nil)

	f := &wasm.FunctionInstance{
		GoFunc: &hostFn,
		Kind:   wasm.FunctionKindGoModule,
		Type:   sig,
		Module: module,
		Index:  0,
	}

	me, err := e.NewModuleEngine(t.Name(), m, nil, []*wasm.FunctionInstance{f}, nil, nil)
	require.NoError(t, err)

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := me.Call(testCtx, modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, mMemory)
	})
}

func RunTestModuleEngine_Call_HostFn(t *testing.T, et EngineTester) {
	runTestModuleEngine_Call_HostFn_ModuleContext(t, et) // TODO: refactor to use the same test interface.

	e := et.NewEngine(wasm.Features20191205)

	host, imported, importing, close := setupCallTests(t, e)
	defer close()

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	tests := []struct {
		name   string
		module *wasm.CallContext
		fn     *wasm.FunctionInstance
	}{
		{
			name:   wasmFnName,
			module: imported.CallCtx,
			fn:     imported.Exports[wasmFnName].Function,
		},
		{
			name:   hostFnName,
			module: host.CallCtx,
			fn:     host.Exports[hostFnName].Function,
		},
		{
			name:   callHostFnName,
			module: imported.CallCtx,
			fn:     imported.Exports[callHostFnName].Function,
		},
		{
			name:   callImportCallHostFnName,
			module: importing.CallCtx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := tc.module
			f := tc.fn
			results, err := f.Module.Engine.Call(testCtx, m, f, 1)
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])
		})
	}
}

func RunTestModuleEngine_Call_Errors(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	host, imported, importing, close := setupCallTests(t, e)
	defer close()

	tests := []struct {
		name        string
		module      *wasm.CallContext
		fn          *wasm.FunctionInstance
		input       []uint64
		expectedErr string
	}{
		{
			name:        "host function not enough parameters",
			input:       []uint64{},
			module:      host.CallCtx,
			fn:          host.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "host function too many parameters",
			input:       []uint64{1, 2},
			module:      host.CallCtx,
			fn:          host.Exports[hostFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:        "wasm function not enough parameters",
			input:       []uint64{},
			module:      imported.CallCtx,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "wasm function too many parameters",
			input:       []uint64{1, 2},
			module:      imported.CallCtx,
			fn:          imported.Exports[wasmFnName].Function,
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:   "wasm function panics with wasmruntime.Error",
			input:  []uint64{0},
			module: imported.CallCtx,
			fn:     imported.Exports[wasmFnName].Function,
			expectedErr: `wasm error: integer divide by zero
wasm stack trace:
	imported.wasm_div_by(i32) i32`,
		},
		{
			name:   "host function that panics",
			input:  []uint64{math.MaxUint32},
			module: host.CallCtx,
			fn:     host.Exports[hostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32`,
		},
		{
			name:   "host function panics with runtime.Error",
			input:  []uint64{0},
			module: host.CallCtx,
			fn:     host.Exports[hostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: imported.CallCtx,
			fn:     imported.Exports[callHostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.CallCtx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: importing.CallCtx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.CallCtx,
			fn:     importing.Exports[callImportCallHostFnName].Function,
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.host_div_by(i32) i32
	imported.call->host_div_by(i32) i32
	importing.call_import->call->host_div_by(i32) i32`,
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := tc.module
			f := tc.fn
			_, err := f.Module.Engine.Call(testCtx, m, f, tc.input...)
			require.EqualError(t, err, tc.expectedErr)

			// Ensure the module still works
			results, err := f.Module.Engine.Call(testCtx, m, f, 1)
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

func setupCallTests(t *testing.T, e wasm.Engine) (*wasm.ModuleInstance, *wasm.ModuleInstance, *wasm.ModuleInstance, func()) {
	i32 := wasm.ValueTypeI32
	ft := &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}

	hostFnVal := reflect.ValueOf(divBy)
	hostFnModule := &wasm.Module{
		HostFunctionSection: []*reflect.Value{&hostFnVal},
		TypeSection:         []*wasm.FunctionType{ft},
		FunctionSection:     []wasm.Index{0},
		ID:                  wasm.ModuleID{0},
	}

	err := e.CompileModule(testCtx, hostFnModule)
	require.NoError(t, err)
	hostFn := &wasm.FunctionInstance{GoFunc: &hostFnVal, Kind: wasm.FunctionKindGoNoContext, Type: ft}
	hostFnModuleInstance := &wasm.ModuleInstance{Name: "host"}
	addFunction(hostFnModuleInstance, hostFnName, hostFn)
	hostFnME, err := e.NewModuleEngine(hostFnModuleInstance.Name, hostFnModule, nil, hostFnModuleInstance.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(hostFnModuleInstance, hostFnME)

	importedModule := &wasm.Module{
		ImportSection:   []*wasm.Import{{}},
		TypeSection:     []*wasm.FunctionType{ft},
		FunctionSection: []uint32{0, 0},
		CodeSection: []*wasm.Code{
			{Body: wasmFnBody},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, byte(0), // Calling imported host function ^.
				wasm.OpcodeEnd}},
		},
		ID: wasm.ModuleID{1},
	}

	err = e.CompileModule(testCtx, importedModule)
	require.NoError(t, err)

	// To use the function, we first need to add it to a module.
	imported := &wasm.ModuleInstance{Name: "imported"}
	addFunction(imported, wasmFnName, getFunctionInstance(importedModule, 0, imported))
	callHostFn := getFunctionInstance(importedModule, 1, imported)
	addFunction(imported, callHostFnName, callHostFn)

	// Compile the imported module
	importedMe, err := e.NewModuleEngine(imported.Name, importedModule, hostFnModuleInstance.Functions, imported.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(imported, importedMe)

	// To test stack traces, call the same function from another module
	importingModule := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{ft},
		FunctionSection: []uint32{0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0 /* only one imported function */, wasm.OpcodeEnd}},
		},
		ImportSection: []*wasm.Import{{}},
		ID:            wasm.ModuleID{2},
	}
	err = e.CompileModule(testCtx, importingModule)
	require.NoError(t, err)

	// Add the exported function.
	importing := &wasm.ModuleInstance{Name: "importing"}
	addFunction(importing, callImportCallHostFnName, getFunctionInstance(importedModule, 0, importing))

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importing.Name, importingModule, []*wasm.FunctionInstance{callHostFn}, importing.Functions, nil, nil)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)

	// Add the imported functions back to the importing module.
	importing.Functions = append([]*wasm.FunctionInstance{callHostFn}, importing.Functions...)

	return hostFnModuleInstance, imported, importing, func() {
		e.DeleteCompiledModule(hostFnModule)
		e.DeleteCompiledModule(importedModule)
		e.DeleteCompiledModule(importingModule)
	}
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
	module.CallCtx = wasm.NewCallContext(nil, module, nil)
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
