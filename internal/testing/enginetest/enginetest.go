// Package enginetest contains tests common to any internalwasm.Engine implementation. Defining these as top-level
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
// Note: These tests intentionally avoid using internalwasm.Store as it is important to know both the dependencies and
// the capabilities at the internalwasm.Engine abstraction.
package enginetest

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

func RunTestModuleEngine_Call(t *testing.T, newEngine func() wasm.Engine) {
	e := newEngine()

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	i64 := wasm.ValueTypeI64
	fn := &wasm.FunctionInstance{
		Name: "fn",
		Kind: wasm.FunctionKindWasm,
		Type: &wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}},
		Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd},
	}

	// To use the function, we first need to add it to a module.
	module := &wasm.ModuleInstance{Name: t.Name()}
	addFunction(module, fn)

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, nil, module.Functions, nil, nil)
	require.NoError(t, err)
	defer me.Close()

	// Create a call context which links the module to the module-engine compiled from it.
	ctx := newModuleContext(module, me)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	results, err := me.Call(ctx, fn, 3)
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

func RunTestEngine_NewModuleEngine_InitTable(t *testing.T, initTable func(me wasm.ModuleEngine, initTableLen uint32, initTableIdxToFnIdx map[wasm.Index]wasm.Index) []interface{}, newEngine func() wasm.Engine) {
	e := newEngine()

	t.Run("no table elements", func(t *testing.T) {
		table := &wasm.TableInstance{Min: 2, Table: make([]interface{}, 2)}
		var importedFunctions []*wasm.FunctionInstance
		var moduleFunctions []*wasm.FunctionInstance
		var tableInit map[wasm.Index]wasm.Index

		// Instantiate the module, which has nothing but an empty table.
		me, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer me.Close()

		// Since there are no elements to initialize, we expect the table to be nil.
		require.Equal(t, table.Table, make([]interface{}, 2))
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
		defer me.Close()

		// The functions mapped to the table are defined in the same moduleEngine
		require.Equal(t, table.Table, initTable(me, table.Min, tableInit))
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
		defer imported.Close()

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer imported.Close()

		// A moduleEngine's compiled function slice includes its imports, so the offsets is absolute.
		require.Equal(t, table.Table, initTable(importing, table.Min, tableInit))
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
		defer imported.Close()

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importedFunctions, moduleFunctions, table, tableInit)
		require.NoError(t, err)
		defer importing.Close()

		// A moduleEngine's compiled function slice includes its imports, so the offsets are absolute.
		require.Equal(t, table.Table, initTable(importing, table.Min, tableInit))
	})
}

func RunTestModuleEngine_Call_HostFn(t *testing.T, newEngine func() wasm.Engine) {
	memory := &wasm.MemoryInstance{}
	var ctxMemory publicwasm.Memory
	hostFn := reflect.ValueOf(func(ctx publicwasm.Module, v uint64) uint64 {
		ctxMemory = ctx.Memory()
		return v
	})

	e := newEngine()
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

	t.Run("defaults to module memory when call stack empty", func(t *testing.T) {
		// When calling a host func directly, there may be no stack. This ensures the module's memory is used.
		results, err := me.Call(modCtx, f, 3)
		require.NoError(t, err)
		require.Equal(t, uint64(3), results[0])
		require.Same(t, memory, ctxMemory)
	})

	t.Run("errs when not enough parameters", func(t *testing.T) {
		_, err := me.Call(modCtx, f)
		require.EqualError(t, err, "expected 1 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		_, err := me.Call(modCtx, f, 1, 2)
		require.EqualError(t, err, "expected 1 params, but passed 2")
	})
}

// addFunction assigns and adds a function to the module.
func addFunction(module *wasm.ModuleInstance, fn *wasm.FunctionInstance) {
	module.Functions = append(module.Functions, fn)
	// This link is essential for all engines. For example, functions call other functions defined in the same module.
	fn.Module = module
}

// newModuleContext creates an internalwasm.ModuleContext for unit tests.
//
// Note: This sets fields that are not needed in the interpreter, but are required by code compiled by JIT. If a new
// test here passes in the interpreter and segmentation faults in JIT, check for a new field offset or a change in JIT
// (ex. jit.TestVerifyOffsetValue). It is possible for all other tests to pass as that field is implicitly set by
// internalwasm.Store: store isn't used here for unit test precision.
func newModuleContext(module *wasm.ModuleInstance, engine wasm.ModuleEngine) *wasm.ModuleContext {
	// moduleInstanceEngineOffset
	module.Engine = engine
	// callEngineModuleContextModuleInstanceAddressOffset
	return wasm.NewModuleContext(context.Background(), nil, module, nil)
}
