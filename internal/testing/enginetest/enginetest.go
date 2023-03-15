// Package enginetest contains tests common to any wasm.Engine implementation. Defining these as top-level
// functions is less burden than copy/pasting the implementations, while still allowing test caching to operate.
//
// In simplest case, dispatch:
//
//	func TestModuleEngine_Call(t *testing.T) {
//		enginetest.RunTestModuleEngine_Call(t, NewEngine)
//	}
//
// Some tests using the Compiler Engine may need to guard as they use compiled features:
//
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
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

const (
	i32, i64 = wasm.ValueTypeI32, wasm.ValueTypeI64
)

var (
	// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
	testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")
	// v_v is a nullary function type (void -> void)
	v_v = wasm.FunctionType{}
)

type EngineTester interface {
	// IsCompiler returns true if this engine is a compiler.
	IsCompiler() bool

	NewEngine(enabledFeatures api.CoreFeatures) wasm.Engine

	ListenerFactory() experimental.FunctionListenerFactory

	// CompiledFunctionPointerValue returns the opaque compiledFunction's pointer for the `funcIndex`.
	CompiledFunctionPointerValue(tme wasm.ModuleEngine, funcIndex wasm.Index) uint64
}

// RunTestEngine_MemoryGrowInRecursiveCall ensures that it's safe to grow memory in the recursive Wasm calls.
func RunTestEngine_MemoryGrowInRecursiveCall(t *testing.T, et EngineTester) {
	enabledFeatures := api.CoreFeaturesV1
	e := et.NewEngine(enabledFeatures)
	s := wasm.NewStore(enabledFeatures, e)

	const hostModuleName = "env"
	const hostFnName = "grow_memory"
	var growFn api.Function
	hm, err := wasm.NewHostModule(hostModuleName, map[string]interface{}{hostFnName: func() {
		// Does the recursive call into Wasm, which grows memory.
		_, err := growFn.Call(context.Background())
		require.NoError(t, err)
	}}, map[string]*wasm.HostFuncNames{hostFnName: {}}, enabledFeatures)
	require.NoError(t, err)

	err = s.Engine.CompileModule(testCtx, hm, nil, false)
	require.NoError(t, err)

	typeIDs, err := s.GetFunctionTypeIDs(hm.TypeSection)
	require.NoError(t, err)

	_, err = s.Instantiate(testCtx, hm, hostModuleName, nil, typeIDs)
	require.NoError(t, err)

	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{}, Results: []wasm.ValueType{}}},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			{
				Body: []byte{
					// Calls the imported host function, which in turn calls the next in-Wasm function recursively.
					wasm.OpcodeCall, 0,
					// Access the memory and this should succeed as we already had memory grown at this point.
					wasm.OpcodeI32Const, 0,
					wasm.OpcodeI32Load, 0x2, 0x0,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				},
			},
			{
				// Grows memory by 1 page.
				Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeMemoryGrow, wasm.OpcodeDrop, wasm.OpcodeEnd},
			},
		},
		MemorySection: &wasm.Memory{Max: 1000},
		ImportSection: []wasm.Import{{Module: hostModuleName, Name: hostFnName, DescFunc: 0}},
	}
	m.BuildFunctionDefinitions()
	m.BuildMemoryDefinitions()

	err = s.Engine.CompileModule(testCtx, m, nil, false)
	require.NoError(t, err)

	typeIDs, err = s.GetFunctionTypeIDs(m.TypeSection)
	require.NoError(t, err)

	inst, err := s.Instantiate(testCtx, m, t.Name(), nil, typeIDs)
	require.NoError(t, err)

	growFn = inst.Function(2)
	_, err = inst.Function(1).Call(context.Background())
	require.NoError(t, err)
}

func RunTestEngine_NewModuleEngine(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV1)

	t.Run("error before instantiation", func(t *testing.T) {
		_, err := e.NewModuleEngine("mymod", &wasm.Module{}, nil)
		require.EqualError(t, err, "source module for mymod must be compiled before instantiation")
	})

	t.Run("sets module name", func(t *testing.T) {
		m := &wasm.Module{}
		err := e.CompileModule(testCtx, m, nil, false)
		require.NoError(t, err)
		me, err := e.NewModuleEngine(t.Name(), m, nil)
		require.NoError(t, err)
		require.Equal(t, t.Name(), me.Name())
	})
}

func RunTestModuleEngine_Call(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV2)

	// Define a basic function which defines two parameters and two results.
	// This is used to test results when incorrect arity is used.
	m := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{
				Params:            []wasm.ValueType{i64, i64},
				Results:           []wasm.ValueType{i64, i64},
				ParamNumInUint64:  2,
				ResultNumInUint64: 2,
			},
		},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeEnd}},
		},
	}

	m.BuildFunctionDefinitions()
	listeners := buildListeners(et.ListenerFactory(), m)
	err := e.CompileModule(testCtx, m, listeners, false)
	require.NoError(t, err)

	// To use the function, we first need to add it to a module.
	module := &wasm.ModuleInstance{Name: t.Name(), TypeIDs: []wasm.FunctionTypeID{0}}
	module.Functions = module.BuildFunctions(m, nil)

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, m, module.Functions)
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	fn := &module.Functions[0]

	ce, err := me.NewCallEngine(module.CallCtx, fn)
	require.NoError(t, err)

	results, err := ce.Call(testCtx, module.CallCtx, []uint64{1, 2})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2}, results)

	t.Run("errs when not enough parameters", func(t *testing.T) {
		ce, err := me.NewCallEngine(module.CallCtx, fn)
		require.NoError(t, err)

		_, err = ce.Call(testCtx, module.CallCtx, nil)
		require.EqualError(t, err, "expected 2 params, but passed 0")
	})

	t.Run("errs when too many parameters", func(t *testing.T) {
		ce, err := me.NewCallEngine(module.CallCtx, fn)
		require.NoError(t, err)

		_, err = ce.Call(testCtx, module.CallCtx, []uint64{1, 2, 3})
		require.EqualError(t, err, "expected 2 params, but passed 3")
	})
}

func RunTestModuleEngine_LookupFunction(t *testing.T, et EngineTester) {
	e := et.NewEngine(api.CoreFeaturesV1)

	mod := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Params: []wasm.ValueType{wasm.ValueTypeV128}}},
		FunctionSection: []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{
				Body: []byte{wasm.OpcodeEnd},
			}, {Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}},
		},
	}

	mod.BuildFunctionDefinitions()
	err := e.CompileModule(testCtx, mod, nil, false)
	require.NoError(t, err)
	m := &wasm.ModuleInstance{TypeIDs: []wasm.FunctionTypeID{0, 1}}
	m.Tables = []*wasm.TableInstance{
		{Min: 2, References: make([]wasm.Reference, 2), Type: wasm.RefTypeFuncref},
		{Min: 2, References: make([]wasm.Reference, 2), Type: wasm.RefTypeExternref},
		{Min: 10, References: make([]wasm.Reference, 10), Type: wasm.RefTypeFuncref},
	}
	m.Functions = m.BuildFunctions(mod, nil)

	me, err := e.NewModuleEngine(m.Name, mod, m.Functions)
	require.NoError(t, err)
	linkModuleToEngine(m, me)

	t.Run("null reference", func(t *testing.T) {
		_, err := me.LookupFunction(m.Tables[0], m.TypeIDs[0], 0) // offset 0 is not initialized yet.
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
		_, err = me.LookupFunction(m.Tables[0], m.TypeIDs[0], 1) // offset 1 is not initialized yet.
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
	})

	m.Tables[0].References[0] = me.FunctionInstanceReference(2)
	m.Tables[0].References[1] = me.FunctionInstanceReference(0)

	t.Run("initialized", func(t *testing.T) {
		index, err := me.LookupFunction(m.Tables[0], m.TypeIDs[0], 0) // offset 0 is now initialized.
		require.NoError(t, err)
		require.Equal(t, wasm.Index(2), index)
		index, err = me.LookupFunction(m.Tables[0], m.TypeIDs[0], 1) // offset 1 is now initialized.
		require.NoError(t, err)
		require.Equal(t, wasm.Index(0), index)
	})

	t.Run("out of range", func(t *testing.T) {
		_, err := me.LookupFunction(m.Tables[0], m.TypeIDs[0], 100 /* out of range */)
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
	})

	t.Run("access to externref table", func(t *testing.T) {
		_, err := me.LookupFunction(m.Tables[1], /* table[1] has externref type. */
			m.TypeIDs[0], 0)
		require.Equal(t, wasmruntime.ErrRuntimeInvalidTableAccess, err)
	})

	t.Run("access to externref table", func(t *testing.T) {
		_, err := me.LookupFunction(m.Tables[0], /* type mismatch */
			m.TypeIDs[1], 0)
		require.Equal(t, wasmruntime.ErrRuntimeIndirectCallTypeMismatch, err)
	})

	m.Tables[2].References[0] = me.FunctionInstanceReference(1)
	m.Tables[2].References[5] = me.FunctionInstanceReference(2)
	t.Run("initialized - tables[2]", func(t *testing.T) {
		index, err := me.LookupFunction(m.Tables[2], m.TypeIDs[0], 0)
		require.NoError(t, err)
		require.Equal(t, wasm.Index(1), index)
		index, err = me.LookupFunction(m.Tables[2], m.TypeIDs[0], 5)
		require.NoError(t, err)
		require.Equal(t, wasm.Index(2), index)
	})
}

func runTestModuleEngine_Call_HostFn_Mem(t *testing.T, et EngineTester, readMem *wasm.Code) {
	e := et.NewEngine(api.CoreFeaturesV1)
	_, importing, done := setupCallMemTests(t, e, readMem, et.ListenerFactory())
	defer done()

	importingMemoryVal := uint64(6)
	importing.Memory = &wasm.MemoryInstance{Buffer: u64.LeBytes(importingMemoryVal), Min: 1, Cap: 1, Max: 1}

	tests := []struct {
		name     string
		fn       *wasm.FunctionInstance
		expected uint64
	}{
		{
			name:     callImportReadMemName,
			fn:       &importing.Functions[importing.Exports[callImportReadMemName].Index],
			expected: importingMemoryVal,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ce, err := tc.fn.Module.Engine.NewCallEngine(tc.fn.Module.CallCtx, tc.fn)
			require.NoError(t, err)

			results, err := ce.Call(testCtx, importing.CallCtx, nil)
			require.NoError(t, err)
			require.Equal(t, tc.expected, results[0])
		})
	}
}

func RunTestModuleEngine_Call_HostFn(t *testing.T, et EngineTester) {
	t.Run("wasm", func(t *testing.T) {
		runTestModuleEngine_Call_HostFn(t, et, hostDivByWasm)
	})
	t.Run("go", func(t *testing.T) {
		runTestModuleEngine_Call_HostFn(t, et, &hostDivByGo)
		runTestModuleEngine_Call_HostFn_Mem(t, et, &hostReadMemGo)
	})
}

func runTestModuleEngine_Call_HostFn(t *testing.T, et EngineTester, hostDivBy *wasm.Code) {
	e := et.NewEngine(api.CoreFeaturesV1)

	_, imported, importing, done := setupCallTests(t, e, hostDivBy, et.ListenerFactory())
	defer done()

	// Ensure the base case doesn't fail: A single parameter should work as that matches the function signature.
	tests := []struct {
		name   string
		module *wasm.CallContext
		fn     *wasm.FunctionInstance
	}{
		{
			name:   divByWasmName,
			module: imported.CallCtx,
			fn:     &imported.Functions[imported.Exports[divByWasmName].Index],
		},
		{
			name:   callDivByGoName,
			module: imported.CallCtx,
			fn:     &imported.Functions[imported.Exports[callDivByGoName].Index],
		},
		{
			name:   callImportCallDivByGoName,
			module: importing.CallCtx,
			fn:     &importing.Functions[importing.Exports[callImportCallDivByGoName].Index],
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := tc.module
			f := tc.fn

			ce, err := f.Module.Engine.NewCallEngine(m, f)
			require.NoError(t, err)

			results, err := ce.Call(testCtx, m, []uint64{1})
			require.NoError(t, err)
			require.Equal(t, uint64(1), results[0])

			results2, err := ce.Call(testCtx, m, []uint64{1})
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

	_, imported, importing, done := setupCallTests(t, e, &hostDivByGo, et.ListenerFactory())
	defer done()

	tests := []struct {
		name        string
		module      *wasm.CallContext
		fn          *wasm.FunctionInstance
		input       []uint64
		expectedErr string
	}{
		{
			name:        "wasm function not enough parameters",
			input:       []uint64{},
			module:      imported.CallCtx,
			fn:          &imported.Functions[imported.Exports[divByWasmName].Index],
			expectedErr: `expected 1 params, but passed 0`,
		},
		{
			name:        "wasm function too many parameters",
			input:       []uint64{1, 2},
			module:      imported.CallCtx,
			fn:          &imported.Functions[imported.Exports[divByWasmName].Index],
			expectedErr: `expected 1 params, but passed 2`,
		},
		{
			name:   "wasm function panics with wasmruntime.Error",
			input:  []uint64{0},
			module: imported.CallCtx,
			fn:     &imported.Functions[imported.Exports[divByWasmName].Index],
			expectedErr: `wasm error: integer divide by zero
wasm stack trace:
	imported.div_by.wasm(i32) i32`,
		},
		{
			name:   "wasm calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: imported.CallCtx,
			fn:     &imported.Functions[imported.Exports[callDivByGoName].Index],
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.CallCtx,
			fn:     &importing.Functions[importing.Exports[callImportCallDivByGoName].Index],
			expectedErr: `runtime error: integer divide by zero (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32
	importing.call_import->call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm that calls host function that panics",
			input:  []uint64{math.MaxUint32},
			module: importing.CallCtx,
			fn:     &importing.Functions[importing.Exports[callImportCallDivByGoName].Index],
			expectedErr: `host-function panic (recovered by wazero)
wasm stack trace:
	host.div_by.go(i32) i32
	imported.call->div_by.go(i32) i32
	importing.call_import->call->div_by.go(i32) i32`,
		},
		{
			name:   "wasm calls imported wasm calls host function panics with runtime.Error",
			input:  []uint64{0},
			module: importing.CallCtx,
			fn:     &importing.Functions[importing.Exports[callImportCallDivByGoName].Index],
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
			m := tc.module
			f := tc.fn

			ce, err := f.Module.Engine.NewCallEngine(m, f)
			require.NoError(t, err)

			_, err = ce.Call(testCtx, m, tc.input)
			require.EqualError(t, err, tc.expectedErr)

			// Ensure the module still works
			results, err := ce.Call(testCtx, m, []uint64{1})
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
	e := et.NewEngine(api.CoreFeaturesV2)

	wasmPhrase := "Well, that'll be the day when you say goodbye."
	wasmPhraseSize := uint32(len(wasmPhrase))

	// Define a basic function which defines one parameter. This is used to test results when incorrect arity is used.
	one := uint32(1)
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}, ParamNumInUint64: 1}, v_v},
		FunctionSection: []wasm.Index{0, 1},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 2},
		DataSection: []wasm.DataSegment{
			{
				Passive: true,
				Init:    []byte(wasmPhrase),
			},
		},
		DataCountSection: &one,
		CodeSection: []wasm.Code{
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
		ExportSection: []wasm.Export{
			{Name: "grow", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "init", Type: wasm.ExternTypeFunc, Index: 1},
		},
	}
	m.BuildFunctionDefinitions()
	listeners := buildListeners(et.ListenerFactory(), m)

	err := e.CompileModule(testCtx, m, listeners, false)
	require.NoError(t, err)

	// Assign memory to the module instance
	module := &wasm.ModuleInstance{
		Name:          t.Name(),
		Memory:        wasm.NewMemoryInstance(m.MemorySection),
		DataInstances: []wasm.DataInstance{m.DataSection[0].Init},
		TypeIDs:       []wasm.FunctionTypeID{0, 1},
	}
	var memory api.Memory = module.Memory

	// To use functions, we need to instantiate them (associate them with a ModuleInstance).
	module.Functions = module.BuildFunctions(m, nil)
	module.BuildExports(m.ExportSection)
	grow, init := &module.Functions[0], &module.Functions[1]

	// Compile the module
	me, err := e.NewModuleEngine(module.Name, m, module.Functions)
	require.NoError(t, err)
	linkModuleToEngine(module, me)

	buf, ok := memory.Read(0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, make([]byte, wasmPhraseSize), buf)

	// Initialize the memory using Wasm. This copies the test phrase.
	initCallEngine, err := me.NewCallEngine(module.CallCtx, init)
	require.NoError(t, err)
	_, err = initCallEngine.Call(testCtx, module.CallCtx, nil)
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
	buf2, ok := memory.Read(0, wasmPhraseSize)
	require.True(t, ok)
	require.Equal(t, buf, buf2)

	// Now, append to the buffer we got from Wasm. As this changes capacity, it should result in a new byte slice.
	buf = append(buf, 'u', 's', '.')
	require.Equal(t, hostPhrase, string(buf))

	// To prove the above, we re-read the memory and should not see the appended bytes (rather zeros instead).
	buf2, ok = memory.Read(0, hostPhraseSize)
	require.True(t, ok)
	hostPhraseTruncated := "Goodbye, cruel world. I'm off to join the circ" + string([]byte{0, 0, 0})
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Now, we need to prove the other direction, that when Wasm changes the capacity, the host's buffer is unaffected.
	growCallEngine, err := me.NewCallEngine(module.CallCtx, grow)
	require.NoError(t, err)
	_, err = growCallEngine.Call(testCtx, module.CallCtx, []uint64{1})
	require.NoError(t, err)

	// The host buffer should still contain the same bytes as before grow
	require.Equal(t, hostPhraseTruncated, string(buf2))

	// Re-initialize the memory in wasm, which overwrites the region.
	initCallEngine2, err := me.NewCallEngine(module.CallCtx, init)
	require.NoError(t, err)
	_, err = initCallEngine2.Call(testCtx, module.CallCtx, nil)
	require.NoError(t, err)

	// The host was not affected because it is a different slice due to "memory.grow" affecting the underlying memory.
	require.Equal(t, hostPhraseTruncated, string(buf2))
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

func setupCallTests(t *testing.T, e wasm.Engine, divBy *wasm.Code, fnlf experimental.FunctionListenerFactory) (*wasm.ModuleInstance, *wasm.ModuleInstance, *wasm.ModuleInstance, func()) {
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
	hostModule.BuildFunctionDefinitions()
	lns := buildListeners(fnlf, hostModule)
	err := e.CompileModule(testCtx, hostModule, lns, false)
	require.NoError(t, err)
	host := &wasm.ModuleInstance{Name: hostModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	host.Functions = host.BuildFunctions(hostModule, nil)
	host.BuildExports(hostModule.ExportSection)
	hostFn := &host.Functions[host.Exports[divByGoName].Index]

	hostME, err := e.NewModuleEngine(host.Name, hostModule, host.Functions)
	require.NoError(t, err)
	linkModuleToEngine(host, hostME)

	importedModule := &wasm.Module{
		ImportSection:   []wasm.Import{{}},
		TypeSection:     []wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0, 0},
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
	importedModule.BuildFunctionDefinitions()
	lns = buildListeners(fnlf, importedModule)
	err = e.CompileModule(testCtx, importedModule, lns, false)
	require.NoError(t, err)

	imported := &wasm.ModuleInstance{Name: importedModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	importedFunctions := imported.BuildFunctions(importedModule, []*wasm.FunctionInstance{hostFn})
	imported.Functions = importedFunctions
	imported.BuildExports(importedModule.ExportSection)
	callHostFn := &imported.Functions[imported.Exports[callDivByGoName].Index]

	// Compile the imported module
	importedMe, err := e.NewModuleEngine(imported.Name, importedModule, importedFunctions)
	require.NoError(t, err)
	linkModuleToEngine(imported, importedMe)

	// To test stack traces, call the same function from another module
	importingModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{ft},
		ImportSection:   []wasm.Import{{}},
		FunctionSection: []wasm.Index{0},
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
	importingModule.BuildFunctionDefinitions()
	lns = buildListeners(fnlf, importingModule)
	err = e.CompileModule(testCtx, importingModule, lns, false)
	require.NoError(t, err)

	// Add the exported function.
	importing := &wasm.ModuleInstance{Name: importingModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	importingFunctions := importing.BuildFunctions(importingModule, []*wasm.FunctionInstance{callHostFn})
	importing.Functions = importingFunctions
	importing.BuildExports(importingModule.ExportSection)

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importing.Name, importingModule, importingFunctions)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)

	return host, imported, importing, func() {
		e.DeleteCompiledModule(hostModule)
		e.DeleteCompiledModule(importedModule)
		e.DeleteCompiledModule(importingModule)
	}
}

func setupCallMemTests(t *testing.T, e wasm.Engine, readMem *wasm.Code, fnlf experimental.FunctionListenerFactory) (*wasm.ModuleInstance, *wasm.ModuleInstance, func()) {
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
	hostModule.BuildFunctionDefinitions()
	err := e.CompileModule(testCtx, hostModule, nil, false)
	require.NoError(t, err)
	host := &wasm.ModuleInstance{Name: hostModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	host.Functions = host.BuildFunctions(hostModule, nil)
	host.BuildExports(hostModule.ExportSection)
	readMemFn := &host.Functions[host.Exports[readMemName].Index]

	hostME, err := e.NewModuleEngine(host.Name, hostModule, host.Functions)
	require.NoError(t, err)
	linkModuleToEngine(host, hostME)

	importingModule := &wasm.Module{
		TypeSection: []wasm.FunctionType{ft},
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
	importingModule.BuildFunctionDefinitions()
	err = e.CompileModule(testCtx, importingModule, nil, false)
	require.NoError(t, err)

	// Add the exported function.
	importing := &wasm.ModuleInstance{Name: importingModule.NameSection.ModuleName, TypeIDs: []wasm.FunctionTypeID{0}}
	importingFunctions := importing.BuildFunctions(importingModule, []*wasm.FunctionInstance{readMemFn})
	// Note: adds imported functions readMemFn and callReadMemFn at index 0 and 1.
	importing.Functions = importingFunctions
	importing.BuildExports(importingModule.ExportSection)

	// Compile the importing module
	importingMe, err := e.NewModuleEngine(importing.Name, importingModule, importingFunctions)
	require.NoError(t, err)
	linkModuleToEngine(importing, importingMe)

	return host, importing, func() {
		e.DeleteCompiledModule(hostModule)
		e.DeleteCompiledModule(importingModule)
	}
}

// linkModuleToEngine assigns fields that wasm.Store would on instantiation. These includes fields both interpreter and
// Compiler needs as well as fields only needed by Compiler.
//
// Note: This sets fields that are not needed in the interpreter, but are required by code compiled by Compiler. If a new
// test here passes in the interpreter and segmentation faults in Compiler, check for a new field offset or a change in Compiler
// (e.g. compiler.TestVerifyOffsetValue). It is possible for all other tests to pass as that field is implicitly set by
// wasm.Store: store isn't used here for unit test precision.
func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me // for Compiler, links the module to the module-engine compiled from it (moduleInstanceEngineOffset).
	// callEngineModuleContextModuleInstanceAddressOffset
	module.CallCtx = wasm.NewCallContext(nil, module, nil)
}

func buildListeners(factory experimental.FunctionListenerFactory, m *wasm.Module) []experimental.FunctionListener {
	if factory == nil || len(m.FunctionSection) == 0 {
		return nil
	}
	listeners := make([]experimental.FunctionListener, len(m.FunctionSection))
	importCount := m.ImportFuncCount()
	for i := 0; i < len(listeners); i++ {
		listeners[i] = factory.NewListener(&m.FunctionDefinitionSection[uint32(i)+importCount])
	}
	return listeners
}
