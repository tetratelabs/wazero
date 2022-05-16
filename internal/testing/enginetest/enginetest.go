// Package enginetest contains tests common to any wasm.Engine implementation. Defining these as top-level
// functions is less burden than copy/pasting the implementations, while still allowing test caching to operate.
//
// Ex. In simplest case, dispatch:
//	func TestModuleEngine_Call(t *testing.T) {
//		enginetest.RunTestModuleEngine_Call(t, NewEngine)
//	}
//
// Ex. Some tests using the Compiler Engine may need to guard as they use compiled features:
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
	// InitTables returns expected table contents ([]wasm.Reference) per table.
	InitTables(me wasm.ModuleEngine, tableIndexToLen map[wasm.Index]int,
		tableInits []wasm.TableInitEntry) [][]wasm.Reference
	// CompiledFunctionPointerValue returns the opaque compiledFunction's pointer for the `funcIndex`.
	CompiledFunctionPointerValue(tme wasm.ModuleEngine, funcIndex wasm.Index) uint64
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

func RunTestEngine_InitializeFuncrefGlobals(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20220419)

	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}},
		FunctionSection: []uint32{0, 0, 0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}, LocalTypes: []wasm.ValueType{wasm.ValueTypeI64}},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}, LocalTypes: []wasm.ValueType{wasm.ValueTypeI64}},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}, LocalTypes: []wasm.ValueType{wasm.ValueTypeI64}},
		},
	}

	err := e.CompileModule(testCtx, m)
	require.NoError(t, err)

	// To use the function, we first need to add it to a module.
	var fns []*wasm.FunctionInstance
	for i := range m.CodeSection {
		typeIndex := m.FunctionSection[i]
		f := &wasm.FunctionInstance{
			Kind:       wasm.FunctionKindWasm,
			Type:       m.TypeSection[typeIndex],
			Body:       m.CodeSection[i].Body,
			LocalTypes: m.CodeSection[i].LocalTypes,
			Idx:        wasm.Index(i),
		}
		fns = append(fns, f)
	}

	me, err := e.NewModuleEngine(t.Name(), m, nil, fns, nil, nil)
	require.NoError(t, err)

	nullRefVal := wasm.GlobalInstanceNullFuncRefValue
	globals := []*wasm.GlobalInstance{
		{Val: 10, Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32}},
		{Val: uint64(nullRefVal), Type: &wasm.GlobalType{ValType: wasm.ValueTypeFuncref}},
		{Val: uint64(2), Type: &wasm.GlobalType{ValType: wasm.ValueTypeFuncref}},
		{Val: uint64(1), Type: &wasm.GlobalType{ValType: wasm.ValueTypeFuncref}},
		{Val: uint64(0), Type: &wasm.GlobalType{ValType: wasm.ValueTypeFuncref}},
	}
	me.InitializeFuncrefGlobals(globals)

	// Non-funcref values must be intact.
	require.Equal(t, uint64(10), globals[0].Val)
	// The second global had wasm.GlobalInstanceNullFuncRefValue, so that value must be translated as null reference (uint64(0)).
	require.Zero(t, globals[1].Val)
	// Non GlobalInstanceNullFuncRefValue valued globals must result in having the valid compiled function's pointers.
	require.Equal(t, et.CompiledFunctionPointerValue(me, 2), globals[2].Val)
	require.Equal(t, et.CompiledFunctionPointerValue(me, 1), globals[3].Val)
	require.Equal(t, et.CompiledFunctionPointerValue(me, 0), globals[4].Val)
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
		Idx:        index + c,
	}
}

func RunTestModuleEngine_Call(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20191205)

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	i64 := wasm.ValueTypeI64
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}, ParamNumInUint64: 1, ResultNumInUint64: 1}},
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
		_, err = e.NewModuleEngine(t.Name(), m, nil, nil, []*wasm.TableInstance{table}, nil)
		require.NoError(t, err)

		// Since there are no elements to initialize, we expect the table to be nil.
		require.Equal(t, table.References, make([]wasm.Reference, 2))
	})
	t.Run("module-defined function", func(t *testing.T) {
		tables := []*wasm.TableInstance{
			{Min: 2, References: make([]wasm.Reference, 2)},
			{Min: 10, References: make([]wasm.Reference, 10)},
		}

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

		var func1, func2 = uint32(2), uint32(1)
		tableInits := []wasm.TableInitEntry{
			{TableIndex: 0, Offset: 0, FunctionIndexes: []*wasm.Index{&func1}},
			{TableIndex: 1, Offset: 5, FunctionIndexes: []*wasm.Index{&func2}},
		}

		// Instantiate the module whose table points to its own functions.
		me, err := e.NewModuleEngine(t.Name(), m, nil, moduleFunctions, tables, tableInits)
		require.NoError(t, err)

		// The functions mapped to the table are defined in the same moduleEngine
		expectedTables := et.InitTables(me, map[wasm.Index]int{0: 2, 1: 10}, tableInits)
		for idx, table := range tables {
			require.Equal(t, expectedTables[idx], table.References)
		}
	})

	t.Run("imported function", func(t *testing.T) {
		tables := []*wasm.TableInstance{{Min: 2, References: make([]wasm.Reference, 2)}}

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

		f := uint32(2)
		tableInits := []wasm.TableInitEntry{
			{TableIndex: 0, Offset: 0, FunctionIndexes: []*wasm.Index{&f}},
		}

		importing, err := e.NewModuleEngine(t.Name(), importingModule, importedFunctions, moduleFunctions, tables, tableInits)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets is absolute.
		expectedTables := et.InitTables(importing, map[wasm.Index]int{0: 2}, tableInits)
		for idx, table := range tables {
			require.Equal(t, expectedTables[idx], table.References)
		}
	})

	t.Run("mixed functions", func(t *testing.T) {
		tables := []*wasm.TableInstance{{Min: 2, References: make([]wasm.Reference, 2)}}

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

		var func1, func2 = uint32(0), uint32(4)
		tableInits := []wasm.TableInitEntry{
			{TableIndex: 0, Offset: 0, FunctionIndexes: []*wasm.Index{&func1, &func2}},
		}

		// Instantiate the importing module, which is whose table is initialized.
		importing, err := e.NewModuleEngine(t.Name(), importingModule, importedFunctions, moduleFunctions, tables, tableInits)
		require.NoError(t, err)

		// A moduleEngine's compiled function slice includes its imports, so the offsets are absolute.
		expectedTables := et.InitTables(importing, map[wasm.Index]int{0: 2}, tableInits)
		for idx, table := range tables {
			require.Equal(t, expectedTables[idx], table.References)
		}
	})
}

func runTestModuleEngine_Call_HostFn_ModuleContext(t *testing.T, et EngineTester) {
	features := wasm.Features20191205
	e := et.NewEngine(features)

	sig := &wasm.FunctionType{
		Params:           []wasm.ValueType{wasm.ValueTypeI64},
		Results:          []wasm.ValueType{wasm.ValueTypeI64},
		ParamNumInUint64: 1, ResultNumInUint64: 1,
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
		Idx:    0,
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

// RunTestModuleEngine_Memory shows that the byte slice returned from api.Memory Read is not a copy, rather a re-slice
// of the underlying memory. This allows both host and Wasm to see each other's writes, unless one side changes the
// capacity of the slice.
//
// Known cases that change the slice capacity:
// * Host code calls append on a byte slice returned by api.Memory Read
// * Wasm code calls wasm.OpcodeMemoryGrowName and this changes the capacity (by default, it will).
func RunTestModuleEngine_Memory(t *testing.T, et EngineTester) {
	e := et.NewEngine(wasm.Features20220419)

	wasmPhrase := "Well, that'll be the day when you say goodbye."
	wasmPhraseSize := uint32(len(wasmPhrase))

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	one := uint32(1)
	m := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}, ParamNumInUint64: 1}, {}},
		FunctionSection: []wasm.Index{0, 1},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 2},
		DataSection: []*wasm.DataSegment{
			{
				OffsetExpression: nil, // passive
				Init:             []byte(wasmPhrase),
			},
		},
		DataCountSection: &one,
		CodeSection: []*wasm.Code{
			{Body: []byte{ // "grow"
				wasm.OpcodeLocalGet, 0, // how many pages to grow (param)
				wasm.OpcodeMemoryGrow, 0, // memory index zero
				wasm.OpcodeDrop, // drop the previous page count (or -1 if grow failed)
				wasm.OpcodeEnd,
			}},
			{Body: []byte{ // "init"
				wasm.OpcodeI32Const, 0, // target offset
				wasm.OpcodeI32Const, 0, // source offset
				wasm.OpcodeI32Const, byte(wasmPhraseSize), // len
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryInit, 0, 0, // segment 0, memory 0
				wasm.OpcodeEnd,
			}},
		},
	}
	// Compile the Wasm into wazeroir
	err := e.CompileModule(testCtx, m)
	require.NoError(t, err)

	// Assign memory to the module instance
	module := &wasm.ModuleInstance{
		Name:          t.Name(),
		Memory:        wasm.NewMemoryInstance(m.MemorySection),
		DataInstances: []wasm.DataInstance{m.DataSection[0].Init},
	}
	var memory api.Memory = module.Memory

	// To use functions, we need to instantiate them (associate them with a ModuleInstance).
	grow := getFunctionInstance(m, 0, module)
	addFunction(module, "grow", grow)
	init := getFunctionInstance(m, 1, module)
	addFunction(module, "init", init)

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, m, nil, module.Functions, nil, nil)
	init.Module.Engine = me
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	buf, ok := memory.Read(testCtx, 0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, make([]byte, wasmPhraseSize), buf)

	// Initialize the memory using Wasm. This copies the test phrase.
	_, err = me.Call(testCtx, module.CallCtx, init)
	require.NoError(t, err)

	// We expect the same []byte read earlier to now include the phrase in wasm.
	require.Equal(t, wasmPhrase, string(buf))

	hostPhrase := "Goodbye, cruel world. I'm off to join the circus." // Intentionally slightly longer.
	hostPhraseSize := uint32(len(hostPhrase))

	// Copy over the buffer, which should stop at the current length.
	copy(buf, hostPhrase)
	require.Equal(t, "Goodbye, cruel world. I'm off to join the circ", string(buf))

	// The underlying memory should be updated. This proves that Memory.Read returns a re-slice, not a copy, and that
	// programs can rely on this (for example, to update shared state in Wasm and view that in Go and visa versa).
	buf2, ok := memory.Read(testCtx, 0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, buf, buf2)

	// Now, append to the buffer we got from Wasm. As this changes capacity, it should result in a new byte slice.
	buf = append(buf, 'u', 's', '.')
	require.Equal(t, hostPhrase, string(buf))

	// To prove the above, we re-read the memory and should not see the appended bytes (rather zeros instead).
	buf2, ok = memory.Read(testCtx, 0, hostPhraseSize)
	require.True(t, ok)
	hostPhraseTruncated := "Goodbye, cruel world. I'm off to join the circ" + string([]byte{0, 0, 0})
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Now, we need to prove the other direction, that when Wasm changes the capacity, the host's buffer is unaffected.
	_, err = me.Call(testCtx, module.CallCtx, grow, 1)
	require.NoError(t, err)

	// The host buffer should still contain the same bytes as before grow
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Re-initialize the memory in wasm, which overwrites the region.
	_, err = me.Call(testCtx, module.CallCtx, init)
	require.NoError(t, err)

	// The host was not affected because it is a different slice due to "memory.grow" affecting the underlying memory.
	require.Equal(t, hostPhraseTruncated, string(buf2))
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
	ft := &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}, ParamNumInUint64: 1, ResultNumInUint64: 1}

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
// Compiler needs as well as fields only needed by Compiler.
//
// Note: This sets fields that are not needed in the interpreter, but are required by code compiled by Compiler. If a new
// test here passes in the interpreter and segmentation faults in Compiler, check for a new field offset or a change in Compiler
// (ex. compiler.TestVerifyOffsetValue). It is possible for all other tests to pass as that field is implicitly set by
// wasm.Store: store isn't used here for unit test precision.
func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me // for Compiler, links the module to the module-engine compiled from it (moduleInstanceEngineOffset).
	// callEngineModuleContextModuleInstanceAddressOffset
	module.CallCtx = wasm.NewCallContext(nil, module, nil)
}

// addFunction assigns and adds a function to the module.
func addFunction(module *wasm.ModuleInstance, funcName string, fn *wasm.FunctionInstance) {
	fn.DebugName = wasmdebug.FuncName(module.Name, funcName, fn.Idx)
	module.Functions = append(module.Functions, fn)
	if module.Exports == nil {
		module.Exports = map[string]*wasm.ExportInstance{}
	}
	module.Exports[funcName] = &wasm.ExportInstance{Type: wasm.ExternTypeFunc, Function: fn}
	// This link is essential for all engines. For example, functions call other functions defined in the same module.
	fn.Module = module
}
