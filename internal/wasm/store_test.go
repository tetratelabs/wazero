package internalwasm

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestModuleInstance_Memory(t *testing.T) {
	tests := []struct {
		name        string
		input       *Module
		expected    bool
		expectedLen uint32
	}{
		{
			name:  "no memory",
			input: &Module{},
		},
		{
			name:  "memory not exported",
			input: &Module{MemorySection: []*MemoryType{{1, nil}}},
		},
		{
			name:  "memory not exported, one page",
			input: &Module{MemorySection: []*MemoryType{{1, nil}}},
		},
		{
			name: "memory exported, different name",
			input: &Module{
				MemorySection: []*MemoryType{{1, nil}},
				ExportSection: map[string]*Export{"momory": {Type: ExternTypeMemory, Name: "momory", Index: 0}},
			},
		},
		{
			name: "memory exported, but zero length",
			input: &Module{
				MemorySection: []*MemoryType{{0, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected: true,
		},
		{
			name: "memory exported, one page",
			input: &Module{
				MemorySection: []*MemoryType{{1, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536,
		},
		{
			name: "memory exported, two pages",
			input: &Module{
				MemorySection: []*MemoryType{{2, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536 * 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			s := newStore()

			instance, err := s.Instantiate(tc.input, "test")
			require.NoError(t, err)

			mem := instance.Memory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

func TestPublicModule_String(t *testing.T) {
	s := newStore()

	// Ensure paths that can create the host module can see the name.
	m, err := s.Instantiate(&Module{}, "module")
	require.NoError(t, err)
	require.Equal(t, "Module[module]", m.String())
	require.Equal(t, "Module[module]", s.Module(m.instance.Name).String())
}

func TestStore_ReleaseModule(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	for _, tc := range []struct {
		name        string
		initializer func(t *testing.T, s *Store)
	}{
		{
			name: "Module imports HostModule",
			initializer: func(t *testing.T, s *Store) {
				_, err := s.NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.ModuleContext) {}})
				require.NoError(t, err)
			},
		},
		{
			name: "Module imports Moudle",
			initializer: func(t *testing.T, s *Store) {
				_, err := s.Instantiate(&Module{
					TypeSection:     []*FunctionType{{}},
					FunctionSection: []uint32{0},
					CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
					ExportSection:   map[string]*Export{"fn": {Type: ExternTypeFunc, Index: 0, Name: "fn"}},
				}, importedModuleName)
				require.NoError(t, err)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newStore()
			tc.initializer(t, s)

			_, err := s.Instantiate(&Module{
				TypeSection:   []*FunctionType{{}},
				ImportSection: []*Import{{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0}},
				MemorySection: []*MemoryType{{1, nil}},
				GlobalSection: []*Global{{Type: &GlobalType{}, Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}}},
				TableSection:  []*TableType{{Limit: &LimitsType{Min: 10}}},
			}, importingModuleName)
			require.NoError(t, err)

			// We shouldn't be able to release the imported module as it is in use!
			require.Error(t, s.ReleaseModule(importedModuleName))

			// Can release the importing module
			require.NoError(t, s.ReleaseModule(importingModuleName))
			require.Nil(t, s.modules[importingModuleName])
			require.NotContains(t, s.modules, importingModuleName)

			// Can re-release the importing module
			require.NoError(t, s.ReleaseModule(importingModuleName))

			// Now we should be able to release the imported module.
			require.NoError(t, s.ReleaseModule(importedModuleName))
			require.Nil(t, s.modules[importedModuleName])
			require.NotContains(t, s.modules, importedModuleName)

			// At this point, everything should be freed.
			require.Len(t, s.modules, 0)
			for _, m := range s.memories {
				require.Nil(t, m)
			}
			for _, table := range s.tables {
				require.Nil(t, table)
			}
			for _, f := range s.functions {
				require.Nil(t, f)
			}
			for _, g := range s.globals {
				require.Nil(t, g)
			}

			// One function, globa, memory and table instance was created and freed,
			// therefore, one released index must be captured by store.
			require.Len(t, s.releasedFunctionIndex, 1)
			require.Len(t, s.releasedTableIndex, 1)
			require.Len(t, s.releasedMemoryIndex, 1)
			require.Len(t, s.releasedGlobalIndex, 1)
		})
	}
}

func TestStore_concurrent(t *testing.T) {
	const importedModuleName = "imported"
	const goroutines = 1000

	var wg sync.WaitGroup

	s := newStore()
	_, err := s.NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.ModuleContext) {}})
	require.NoError(t, err)

	hm, ok := s.modules[importedModuleName]
	require.True(t, ok)

	importingModule := &Module{
		TypeSection:     []*FunctionType{{}},
		FunctionSection: []uint32{0},
		CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
		MemorySection:   []*MemoryType{{1, nil}},
		GlobalSection:   []*Global{{Type: &GlobalType{}, Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}}},
		TableSection:    []*TableType{{Limit: &LimitsType{Min: 10}}},
		ImportSection: []*Import{
			{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
		},
	}

	// Concurrent instantiation.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := s.Instantiate(importingModule, strconv.Itoa(i))
			require.NoError(t, err)

			if i == goroutines/2 {
				// Trying to release the imported module concurrently, but should fail as it's in use.
				err := s.ReleaseModule(importedModuleName)
				require.Error(t, err)
			}
		}(i)

		// TODO: in addition to the normal instantiation, we should try to instantiate host module in conjunction.
		// after making store.NewHostModule goroutine-safe.
	}
	wg.Wait()

	// At this point 1000 modules import host modules.
	require.Equal(t, goroutines, hm.dependentCount)

	require.Len(t, s.functions, goroutines+1) // Instantiated + imported one.
	for _, f := range s.functions {
		require.NotNil(t, f)
	}

	require.Len(t, s.tables, goroutines)
	for _, table := range s.tables {
		require.NotNil(t, table)
	}

	require.Len(t, s.globals, goroutines)
	for _, g := range s.globals {
		require.NotNil(t, g)
	}

	require.Len(t, s.memories, goroutines)
	for _, m := range s.memories {
		require.NotNil(t, m)
	}

	// Concurrent release.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			err := s.ReleaseModule(strconv.Itoa(i))
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// No all the importing instances were released, the imported module can be freed.
	require.Zero(t, hm.dependentCount)
	require.NoError(t, s.ReleaseModule(hm.Name))

	// All instances are freed.
	require.Len(t, s.modules, 0)
}

func TestSotre_Instantiate_Errors(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	t.Run("fail resolve import", func(t *testing.T) {
		s := newStore()
		_, err := s.NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.ModuleContext) {}})
		require.NoError(t, err)

		hm := s.modules[importedModuleName]
		require.NotNil(t, hm)

		_, err = s.Instantiate(&Module{
			TypeSection: []*FunctionType{{}},
			ImportSection: []*Import{
				// The fisrt import resolve succeeds -> increment hm.dependentCount.
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
				// But the second one tries to import uninitialized-module ->
				{Type: ExternTypeFunc, Module: "non-exist", Name: "fn", DescFunc: 0},
			},
		}, importingModuleName)
		require.EqualError(t, err, "module \"non-exist\" not instantiated")

		// hm.dependentCount must be intact as the instantiation failed.
		require.Zero(t, hm.dependentCount)
	})

	t.Run("compilation failed", func(t *testing.T) {
		s := newStore()
		catch := s.engine.(*catchContext)
		catch.compilationFailIndex = 3

		_, err := s.NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.ModuleContext) {}})
		require.NoError(t, err)

		hm := s.modules[importedModuleName]
		require.NotNil(t, hm)

		_, err = s.Instantiate(&Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []uint32{0, 0, 0 /* compilation failing function */},
			CodeSection: []*Code{
				{Body: []byte{OpcodeEnd}}, // FunctionIndex = 1
				{Body: []byte{OpcodeEnd}}, // FunctionIndex = 2
				{Body: []byte{OpcodeEnd}}, // FunctionIndex = 3 == compilation failing function.
				// Functions after failrued must not be passed to engine.Release.
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
			},
			ImportSection: []*Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}, importingModuleName)
		require.EqualError(t, err, "compilation failed at index 2/2: compilation failed")

		// hm.dependentCount must be intact as the instantiation failed.
		require.Zero(t, hm.dependentCount)

		require.Equal(t, catch.releasedCalledFunctionIndex, []FunctionIndex{0x1, 0x2})
	})

	t.Run("start func failed", func(t *testing.T) {
		s := newStore()
		catch := s.engine.(*catchContext)
		catch.callFailIndex = 1

		_, err := s.NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.ModuleContext) {}})
		require.NoError(t, err)

		hm := s.modules[importedModuleName]
		require.NotNil(t, hm)

		startFuncIndex := uint32(1)
		_, err = s.Instantiate(&Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []uint32{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
			StartSection:    &startFuncIndex,
			ImportSection: []*Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}, importingModuleName)
		require.EqualError(t, err, "module[test] start function failed: call failed")

		// hm.dependentCount must stay incremented as the instantiation itself has already succeeded.
		require.Equal(t, 1, hm.dependentCount)
	})
}

func TestStore_ExportImportedHostFunction(t *testing.T) {
	s := newStore()

	// Add the host module
	_, err := s.NewHostModule("", map[string]interface{}{"host_fn": func(wasm.ModuleContext) {}})
	require.NoError(t, err)

	t.Run("Module is the importing module", func(t *testing.T) {
		_, err = s.Instantiate(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Type: ExternTypeFunc, Name: "host_fn", DescFunc: 0}},
			MemorySection: []*MemoryType{{1, nil}},
			ExportSection: map[string]*Export{"host.fn": {Type: ExternTypeFunc, Name: "host.fn", Index: 0}},
		}, "test")
		require.NoError(t, err)

		mod, ok := s.modules["test"]
		require.True(t, ok)

		ei, err := mod.getExport("host.fn", ExternTypeFunc)
		require.NoError(t, err)
		// We expect the host function to be called in context of the importing module.
		// Otherwise, it would be the pseudo-module of the host, which only includes types and function definitions.
		// Notably, this ensures the host function call context has the correct memory (from the importing module).
		require.Equal(t, s.modules["test"], ei.Function.Module)
	})
}

func TestFunctionInstance_Call(t *testing.T) {
	type key string
	storeCtx := context.WithValue(context.Background(), key("wa"), "zero")

	notStoreCtx := context.WithValue(context.Background(), key("wazer"), "o")

	tests := []struct {
		name     string
		ctx      context.Context
		expected context.Context
	}{
		{
			name:     "nil defaults to store context",
			ctx:      nil,
			expected: storeCtx,
		},
		{
			name:     "set overrides store context",
			ctx:      notStoreCtx,
			expected: notStoreCtx,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			engine := &catchContext{callFailIndex: -1, compilationFailIndex: -1}
			store := NewStore(storeCtx, engine, Features20191205)

			// Add the host module
			functionName := "fn"
			hm, err := store.NewHostModule("host",
				map[string]interface{}{functionName: func(wasm.ModuleContext) {}},
			)
			require.NoError(t, err)

			// Make a module to import the function
			instantiated, err := store.Instantiate(&Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{{
					Type:     ExternTypeFunc,
					Module:   hm.name,
					Name:     functionName,
					DescFunc: 0,
				}},
				MemorySection: []*MemoryType{{1, nil}},
				ExportSection: map[string]*Export{functionName: {Type: ExternTypeFunc, Name: functionName, Index: 0}},
			}, "test")
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			_, err = instantiated.Function(functionName).Call(tc.ctx)
			require.NoError(t, err)
			if tc.expected == nil {
				require.Nil(t, engine.ctx)
			} else {
				require.Equal(t, tc.expected, engine.ctx.Context())
			}
		})
	}
}

type catchContext struct {
	ctx                                 *ModuleContext
	compilationFailIndex, callFailIndex int
	releasedCalledFunctionIndex         []FunctionIndex
}

func (e *catchContext) Call(ctx *ModuleContext, f *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	if e.callFailIndex >= 0 && f.Index == FunctionIndex(e.callFailIndex) {
		return nil, fmt.Errorf("call failed")
	}
	e.ctx = ctx
	return
}

func (e *catchContext) Compile(f *FunctionInstance) error {
	if e.compilationFailIndex >= 0 && f.Index == FunctionIndex(e.compilationFailIndex) {
		return fmt.Errorf("compilation failed")
	}
	return nil
}

func (e *catchContext) Release(f *FunctionInstance) error {
	e.releasedCalledFunctionIndex = append(e.releasedCalledFunctionIndex, f.Index)
	return nil
}

func TestStore_checkFuncAddrOverflow(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := newStore()
		const max = 10
		s.maximumFunctionIndex = max
		err := s.checkFunctionIndexOverflow(max + 1)
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		s := newStore()
		const max = 10
		s.maximumFunctionIndex = max
		err := s.checkFunctionIndexOverflow(max)
		require.NoError(t, err)
	})
}

func TestStore_getTypeInstance(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := newStore()
		const max = 10
		s.maximumFunctionTypes = max
		s.typeIDs = make(map[string]FunctionTypeID)
		for i := 0; i < max; i++ {
			s.typeIDs[strconv.Itoa(i)] = 0
		}
		_, err := s.getTypeInstance(&FunctionType{})
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		for _, tc := range []*FunctionType{
			{Params: []ValueType{}},
			{Params: []ValueType{ValueTypeF32}},
			{Results: []ValueType{ValueTypeF64}},
			{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		} {
			tc := tc
			t.Run(tc.String(), func(t *testing.T) {
				s := newStore()
				actual, err := s.getTypeInstance(tc)
				require.NoError(t, err)

				expectedTypeID, ok := s.typeIDs[tc.String()]
				require.True(t, ok)
				require.Equal(t, expectedTypeID, actual.TypeID)
				require.Equal(t, tc, actual.Type)
			})
		}
	})
}

func TestExecuteConstExpression(t *testing.T) {
	t.Run("non global expr", func(t *testing.T) {
		for _, vt := range []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64} {
			t.Run(ValueTypeName(vt), func(t *testing.T) {
				// Allocate bytes with enough size for all types.
				expr := &ConstantExpression{Data: make([]byte, 8)}
				switch vt {
				case ValueTypeI32:
					expr.Data[0] = 1
					expr.Opcode = OpcodeI32Const
				case ValueTypeI64:
					expr.Data[0] = 2
					expr.Opcode = OpcodeI64Const
				case ValueTypeF32:
					binary.LittleEndian.PutUint32(expr.Data, math.Float32bits(math.MaxFloat32))
					expr.Opcode = OpcodeF32Const
				case ValueTypeF64:
					binary.LittleEndian.PutUint64(expr.Data, math.Float64bits(math.MaxFloat64))
					expr.Opcode = OpcodeF64Const
				}

				raw := executeConstExpression(nil, expr)
				require.NotNil(t, raw)

				switch vt {
				case ValueTypeI32:
					actual, ok := raw.(int32)
					require.True(t, ok)
					require.Equal(t, int32(1), actual)
				case ValueTypeI64:
					actual, ok := raw.(int64)
					require.True(t, ok)
					require.Equal(t, int64(2), actual)
				case ValueTypeF32:
					actual, ok := raw.(float32)
					require.True(t, ok)
					require.Equal(t, float32(math.MaxFloat32), actual)
				case ValueTypeF64:
					actual, ok := raw.(float64)
					require.True(t, ok)
					require.Equal(t, float64(math.MaxFloat64), actual)
				}
			})
		}
	})
	t.Run("global expr", func(t *testing.T) {
		for _, tc := range []struct {
			valueType ValueType
			val       uint64
		}{
			{valueType: ValueTypeI32, val: 10},
			{valueType: ValueTypeI64, val: 20},
			{valueType: ValueTypeF32, val: uint64(math.Float32bits(634634432.12311))},
			{valueType: ValueTypeF64, val: math.Float64bits(1.12312311)},
		} {
			t.Run(ValueTypeName(tc.valueType), func(t *testing.T) {
				// The index specified in Data equals zero.
				expr := &ConstantExpression{Data: []byte{0}, Opcode: OpcodeGlobalGet}
				globals := []*GlobalInstance{{Val: tc.val, Type: &GlobalType{ValType: tc.valueType}}}

				val := executeConstExpression(globals, expr)
				require.NotNil(t, val)

				switch tc.valueType {
				case ValueTypeI32:
					actual, ok := val.(int32)
					require.True(t, ok)
					require.Equal(t, int32(tc.val), actual)
				case ValueTypeI64:
					actual, ok := val.(int64)
					require.True(t, ok)
					require.Equal(t, int64(tc.val), actual)
				case ValueTypeF32:
					actual, ok := val.(float32)
					require.True(t, ok)
					require.Equal(t, wasm.DecodeF32(tc.val), actual)
				case ValueTypeF64:
					actual, ok := val.(float64)
					require.True(t, ok)
					require.Equal(t, wasm.DecodeF64(tc.val), actual)
				}
			})
		}
	})
}

func TestStore_releaseFunctionInstances(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := FunctionIndex(0)
	maxAddr := FunctionIndex(10)
	s.functions = make([]*FunctionInstance, maxAddr+1)

	s.functions[nonReleaseTargetAddr] = &FunctionInstance{} // Non-nil!

	// Set up existing function instances.
	fs := []*FunctionInstance{{Index: 1}, {Index: 2}, {Index: 3}, {Index: 4}, {Index: maxAddr}}
	for _, f := range fs {
		s.functions[f.Index] = &FunctionInstance{} // Non-nil!
	}

	err := s.releaseFunctions(fs...)
	require.NoError(t, err)

	// Ensure the release targets become nil.
	for _, f := range fs {
		require.Nil(t, s.functions[f.Index])
		require.Contains(t, s.releasedFunctionIndex, f.Index)
	}

	// Plus non-target should remain intact.
	require.NotNil(t, s.functions[nonReleaseTargetAddr])
}

func TestStore_addFunctionInstances(t *testing.T) {
	t.Run("no released index", func(t *testing.T) {
		s := newStore()
		prevMaxAddr := FunctionIndex(10)
		s.functions = make([]*FunctionInstance, prevMaxAddr+1)

		for i := FunctionIndex(0); i < 10; i++ {
			expectedIndex := prevMaxAddr + 1 + i
			f := &FunctionInstance{}
			s.addFunctions(f)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, f.Index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := FunctionIndex(10)
		s.releasedFunctionIndex[expectedAddr] = struct{}{}

		maxAddr := expectedAddr * 10
		tailInstance := &FunctionInstance{}
		s.functions = make([]*FunctionInstance, maxAddr+1)
		s.functions[maxAddr] = tailInstance

		f := &FunctionInstance{}
		s.addFunctions(f)

		// Index must be reused.
		require.Equal(t, expectedAddr, f.Index)
		require.Equal(t, f, s.functions[expectedAddr])

		// And the others must be intact.
		require.Equal(t, tailInstance, s.functions[maxAddr])

		require.Len(t, s.releasedFunctionIndex, 0)
	})
}

func TestStore_releaseGlobalInstances(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := globalIndex(0)
	maxAddr := globalIndex(10)
	s.globals = make([]*GlobalInstance, maxAddr+1)

	s.globals[nonReleaseTargetAddr] = &GlobalInstance{} // Non-nil!

	// Set up existing function instances.
	gs := []*GlobalInstance{{index: 1}, {index: 2}, {index: 3}, {index: 4}, {index: maxAddr}}
	for _, g := range gs {
		s.globals[g.index] = &GlobalInstance{} // Non-nil!
	}

	s.releaseGlobal(gs...)

	// Ensure the release targets become nil.
	for _, g := range gs {
		require.Nil(t, s.globals[g.index])
		require.Contains(t, s.releasedGlobalIndex, g.index)
	}

	// Plus non-target should remain intact.
	require.NotNil(t, s.globals[nonReleaseTargetAddr])
}

func TestStore_addGlobalInstances(t *testing.T) {
	t.Run("no released index", func(t *testing.T) {
		s := newStore()
		prevMaxAddr := globalIndex(10)
		s.globals = make([]*GlobalInstance, prevMaxAddr+1)

		for i := globalIndex(0); i < 10; i++ {
			expectedIndex := prevMaxAddr + 1 + i
			g := &GlobalInstance{}
			s.addGlobals(g)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, g.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := globalIndex(10)
		s.releasedGlobalIndex[expectedAddr] = struct{}{}

		maxAddr := expectedAddr * 10
		tailInstance := &GlobalInstance{}
		s.globals = make([]*GlobalInstance, maxAddr+1)
		s.globals[maxAddr] = tailInstance

		g := &GlobalInstance{}
		s.addGlobals(g)

		// Index must be reused.
		require.Equal(t, expectedAddr, g.index)
		require.Equal(t, g, s.globals[expectedAddr])

		// And the others must be intact.
		require.Equal(t, tailInstance, s.globals[maxAddr])

		require.Len(t, s.releasedGlobalIndex, 0)
	})
}

func TestStore_releaseTableInstance(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := tableIndex(0)
	maxAddr := tableIndex(10)
	s.tables = make([]*TableInstance, maxAddr+1)

	s.tables[nonReleaseTargetAddr] = &TableInstance{} // Non-nil!

	table := &TableInstance{index: 1}

	s.releaseTable(table)

	// Ensure the release targets become nil.
	require.Nil(t, s.tables[table.index])

	require.Contains(t, s.releasedTableIndex, table.index)

	// Plus non-target should remain intact.
	require.NotNil(t, s.tables[nonReleaseTargetAddr])
}

func TestStore_addTableInstance(t *testing.T) {
	t.Run("no released index", func(t *testing.T) {
		s := newStore()
		prevMaxAddr := tableIndex(10)
		s.tables = make([]*TableInstance, prevMaxAddr+1)

		for i := tableIndex(0); i < 10; i++ {
			expectedIndex := prevMaxAddr + 1 + i
			g := &TableInstance{}
			s.addTable(g)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, g.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := tableIndex(10)
		s.releasedTableIndex[expectedAddr] = struct{}{}

		maxAddr := expectedAddr * 10
		tailInstance := &TableInstance{}
		s.tables = make([]*TableInstance, maxAddr+1)
		s.tables[maxAddr] = tailInstance

		table := &TableInstance{}
		s.addTable(table)

		// Index must be reused.
		require.Equal(t, expectedAddr, table.index)
		require.Equal(t, table, s.tables[expectedAddr])

		// And the others must be intact.
		require.Equal(t, tailInstance, s.tables[maxAddr])

		require.Len(t, s.releasedTableIndex, 0)
	})
}

func TestStore_releaseMemoryInstance(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := memoryIndex(0)
	releaseTargetAddr := memoryIndex(10)
	s.memories = make([]*MemoryInstance, releaseTargetAddr+1)

	s.memories[nonReleaseTargetAddr] = &MemoryInstance{} // Non-nil!
	mem := &MemoryInstance{index: releaseTargetAddr}
	s.memories[releaseTargetAddr] = mem // Non-nil!

	s.releaseMemory(mem)

	// Ensure the release targets become nil.
	require.Nil(t, s.memories[mem.index])

	require.Contains(t, s.releasedMemoryIndex, mem.index)

	// Plus non-target should remain intact.
	require.NotNil(t, s.memories[nonReleaseTargetAddr])
}

func TestStore_addMemoryInstance(t *testing.T) {
	t.Run("no released index", func(t *testing.T) {
		s := newStore()
		prevMaxAddr := memoryIndex(10)
		s.memories = make([]*MemoryInstance, prevMaxAddr+1)

		for i := memoryIndex(0); i < 10; i++ {
			expectedIndex := prevMaxAddr + 1 + i
			mem := &MemoryInstance{}
			s.addMemory(mem)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, mem.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := memoryIndex(10)
		s.releasedMemoryIndex[expectedAddr] = struct{}{}

		maxAddr := expectedAddr * 10
		tailInstance := &MemoryInstance{}
		s.memories = make([]*MemoryInstance, maxAddr+1)
		s.memories[maxAddr] = tailInstance

		mem := &MemoryInstance{}
		s.addMemory(mem)

		// Index must be reused.
		require.Equal(t, expectedAddr, mem.index)
		require.Equal(t, mem, s.memories[expectedAddr])

		// And the others must be intact.
		require.Equal(t, tailInstance, s.memories[maxAddr])

		require.Len(t, s.releasedMemoryIndex, 0)
	})
}

func TestStore_resolveImports(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("module not instantiated", func(t *testing.T) {
		s := newStore()
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: "unknown", Name: "unknown"}}})
		require.EqualError(t, err, "module \"unknown\" not instantiated")
	})
	t.Run("export instance not found", func(t *testing.T) {
		s := newStore()
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{}, Name: moduleName}
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: "unknown"}}})
		require.EqualError(t, err, "\"unknown\" is not exported in module \"test\"")
	})
	t.Run("func", func(t *testing.T) {
		t.Run("unknwon type", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 100}}})
			require.EqualError(t, err, "unknown type for function import")
		})
		t.Run("signature mismatch", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Function: &FunctionInstance{Type: &FunctionType{}},
			}}, Name: moduleName}
			m := &Module{
				TypeSection:   []*FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
			}
			_, _, _, _, _, err := s.resolveImports(m)
			require.EqualError(t, err, "signature mimatch: v_f32 != v_v")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			f := &FunctionInstance{Type: &FunctionType{Results: []ValueType{ValueTypeF32}}}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Function: f,
			}}, Name: moduleName}
			m := &Module{
				TypeSection:   []*FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
			}
			functions, _, _, _, moduleImports, err := s.resolveImports(m)
			require.NoError(t, err)
			require.Contains(t, moduleImports, s.modules[moduleName])
			require.Contains(t, functions, f)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
	})
	t.Run("global", func(t *testing.T) {
		t.Run("mutability mismatch", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{Mutable: false}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{Mutable: true}}}})
			require.EqualError(t, err, "incompatible global import: mutability mismatch")
		})
		t.Run("type mismatch", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeF64}}}})
			require.EqualError(t, err, "incompatible global import: value type mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			inst := &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {Type: ExternTypeGlobal, Global: inst}}, Name: moduleName}
			_, globals, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: inst.Type}}})
			require.NoError(t, err)
			require.Contains(t, globals, inst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
	})
	t.Run("table", func(t *testing.T) {
		t.Run("element type", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{ElemType: 0x00}, // Unknown!
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &TableType{ElemType: 0x1}}}})
			require.EqualError(t, err, "incompatible table import: element type mismatch")
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importTableType := &TableType{Limit: &LimitsType{Min: 2}}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Limit.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.EqualError(t, err, "incompatible table import: minimum size mismatch")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importTableType := &TableType{Limit: &LimitsType{Max: &max}}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Limit.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.EqualError(t, err, "incompatible table import: maximum size mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			tableInst := &TableInstance{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: tableInst,
			}}, Name: moduleName}
			_, _, table, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &TableType{Limit: &LimitsType{Max: &max}}}}})
			require.NoError(t, err)
			require.Equal(t, table, tableInst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
	})
	t.Run("memory", func(t *testing.T) {
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importMemoryType := &MemoryType{Min: 2}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{Min: importMemoryType.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.EqualError(t, err, "incompatible memory import: minimum size mismatch")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importMemoryType := &MemoryType{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.EqualError(t, err, "incompatible memory import: maximum size mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			memoryInst := &MemoryInstance{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: memoryInst,
			}}, Name: moduleName}
			_, _, _, memory, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: &MemoryType{Max: &max}}}})
			require.NoError(t, err)
			require.Equal(t, memory, memoryInst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
	})
}

func TestModuleInstance_validateData(t *testing.T) {
	m := &ModuleInstance{Memory: &MemoryInstance{Buffer: make([]byte, 5)}}
	for _, tc := range []struct {
		name   string
		data   []*DataSegment
		expErr bool
	}{
		{
			name: "ok",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}, Init: []byte{0}},
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x2}}, Init: []byte{0}},
			},
		},
		{
			name: "out of bounds - single one byte",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x5}}, Init: []byte{0}},
			},
			expErr: true,
		},
		{
			name: "out of bounds - multi bytes",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x3}}, Init: []byte{0, 1, 2}},
			},
			expErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateData(tc.data)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModuleInstance_applyData(t *testing.T) {
	m := &ModuleInstance{Memory: &MemoryInstance{Buffer: make([]byte, 10)}}
	m.applyData([]*DataSegment{
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []byte{0xa, 0xf}},
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []byte{0x1, 0x5}},
	})
	require.Equal(t, []byte{0xa, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x5}, m.Memory.Buffer)
}

func TestModuleInstance_validateElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		Table:     &TableInstance{Table: make([]TableElement, 10)},
		Functions: make([]*FunctionInstance, 10),
	}
	for _, tc := range []struct {
		name     string
		elements []*ElementSegment
		expErr   bool
	}{
		{
			name: "ok",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "ok on edge",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "out of bounds",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x9}}, Init: []uint32{0, functionCounts - 1}},
			},
			expErr: true,
		},
		{
			name: "unknown function",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts}},
			},
			expErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateElements(tc.elements)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModuleInstance_applyElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		Table:     &TableInstance{Table: make([]TableElement, 10)},
		Functions: make([]*FunctionInstance, 10),
	}
	targetAddr, targetOffset := uint32(1), byte(0)
	targetAddr2, targetOffset2 := functionCounts-1, byte(0x8)
	m.Functions[targetAddr] = &FunctionInstance{Type: &FunctionType{}, Index: FunctionIndex(targetAddr)}
	m.Functions[targetAddr2] = &FunctionInstance{Type: &FunctionType{}, Index: FunctionIndex(targetAddr2)}
	m.applyElements([]*ElementSegment{
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset}}, Init: []uint32{uint32(targetAddr)}},
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset2}}, Init: []uint32{targetAddr2}},
	})
	require.Equal(t, FunctionIndex(targetAddr), m.Table.Table[targetOffset].FunctionIndex)
	require.Equal(t, FunctionIndex(targetAddr2), m.Table.Table[targetOffset2].FunctionIndex)
}

func TestModuleInstance_decImportedCount(t *testing.T) {
	count := 100
	m := ModuleInstance{dependentCount: count}

	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			m.decImportedCount()
		}()
	}
	wg.Wait()
	require.Zero(t, m.dependentCount)
}

func TestModuleInstance_incImportedCount(t *testing.T) {
	count := 100
	m := ModuleInstance{}
	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			m.incImportedCount()
		}()
	}
	wg.Wait()
	require.Equal(t, count, m.dependentCount)
}

func newStore() *Store {
	return NewStore(context.Background(), &catchContext{compilationFailIndex: -1, callFailIndex: -1}, Features20191205)
}
