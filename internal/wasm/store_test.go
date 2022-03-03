package internalwasm

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"strconv"
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

func TestStore_AddHostFunction(t *testing.T) {
	s := newStore()

	hf, err := NewGoFunc("fn", func(wasm.ModuleContext) {})
	require.NoError(t, err)

	// Add the host module
	hostModule := &ModuleInstance{Name: "test", Exports: make(map[string]*ExportInstance, 1)}
	s.moduleInstances[hostModule.Name] = hostModule

	_, err = s.AddHostFunction(hostModule, hf)
	require.NoError(t, err)

	// The function was added to the store, prefixed by the owning module name
	require.Equal(t, 1, len(s.functions))
	fn := s.functions[0]
	require.Equal(t, "test.fn", fn.Name)

	// The function was exported in the module
	require.Equal(t, 1, len(hostModule.Exports))
	exp, ok := hostModule.Exports["fn"]
	require.True(t, ok)

	// Trying to register it again should fail
	_, err = s.AddHostFunction(hostModule, hf)
	require.EqualError(t, err, `"fn" is already exported in module "test"`)

	// Any side effects should be reverted
	require.Equal(t, []*FunctionInstance{fn, nil}, s.functions)
	require.Equal(t, map[string]*ExportInstance{"fn": exp}, hostModule.Exports)
}

func TestStore_ExportImportedHostFunction(t *testing.T) {
	s := newStore()

	hf, err := NewGoFunc("host_fn", func(wasm.ModuleContext) {})
	require.NoError(t, err)

	// Add the host module
	hostModule := &ModuleInstance{Name: "", Exports: make(map[string]*ExportInstance, 1)}
	s.moduleInstances[hostModule.Name] = hostModule
	_, err = s.AddHostFunction(hostModule, hf)
	require.NoError(t, err)

	t.Run("ModuleInstance is the importing module", func(t *testing.T) {
		_, err = s.Instantiate(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Type: ExternTypeFunc, Name: "host_fn", DescFunc: 0}},
			MemorySection: []*MemoryType{{1, nil}},
			ExportSection: map[string]*Export{"host.fn": {Type: ExternTypeFunc, Name: "host.fn", Index: 0}},
		}, "test")
		require.NoError(t, err)

		ei, err := s.getExport("test", "host.fn", ExternTypeFunc)
		require.NoError(t, err)
		os.Environ()
		// We expect the host function to be called in context of the importing module.
		// Otherwise, it would be the pseudo-module of the host, which only includes types and function definitions.
		// Notably, this ensures the host function call context has the correct memory (from the importing module).
		require.Equal(t, s.moduleInstances["test"], ei.Function.ModuleInstance)
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
			engine := &catchContext{}
			store := NewStore(storeCtx, engine, Features20191205)

			// Define a fake host function
			functionName := "fn"
			hostFn := func(ctx wasm.ModuleContext) {
			}
			fn, err := NewGoFunc(functionName, hostFn)
			require.NoError(t, err)

			// Add the host module
			hostModule := &ModuleInstance{Name: "host", Exports: map[string]*ExportInstance{}}
			store.moduleInstances[hostModule.Name] = hostModule
			_, err = store.AddHostFunction(hostModule, fn)
			require.NoError(t, err)

			// Make a module to import the function
			instantiated, err := store.Instantiate(&Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{{
					Type:     ExternTypeFunc,
					Module:   hostModule.Name,
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
	ctx *ModuleContext
}

func (e *catchContext) Call(ctx *ModuleContext, _ *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	e.ctx = ctx
	return
}

func (e *catchContext) Compile(_ *FunctionInstance) error {
	return nil
}

func (e *catchContext) Release(_ *FunctionInstance) error {
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
	var expectedReleasedAddr []FunctionIndex
	fs := []*FunctionInstance{{Index: 1}, {Index: 2}, {Index: 3}, {Index: 4}, {Index: maxAddr}}
	for _, f := range fs {
		s.functions[f.Index] = &FunctionInstance{} // Non-nil!
		expectedReleasedAddr = append(expectedReleasedAddr, f.Index)
	}

	err := s.releaseFunctionInstances(fs...)
	require.NoError(t, err)

	// Ensure the release targets become nil.
	for _, f := range fs {
		require.Nil(t, s.functions[f.Index])
	}

	require.Equal(t, s.releasedFunctionIndex, expectedReleasedAddr)

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
			s.addFunctionInstances(f)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, f.Index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := FunctionIndex(10)
		s.releasedFunctionIndex = []FunctionIndex{1, expectedAddr}
		expectedReleasedAddr := s.releasedFunctionIndex[:1]

		maxAddr := expectedAddr * 10
		tailInstance := &FunctionInstance{}
		s.functions = make([]*FunctionInstance, maxAddr+1)
		s.functions[maxAddr] = tailInstance

		f := &FunctionInstance{}
		s.addFunctionInstances(f)

		// Index must be reused.
		require.Equal(t, expectedAddr, f.Index)

		// And the others must be intact.
		require.Equal(t, tailInstance, s.functions[maxAddr])

		require.Equal(t, expectedReleasedAddr, s.releasedFunctionIndex)
	})
}

func TestStore_releaseGlobalInstances(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := globalIndex(0)
	maxAddr := globalIndex(10)
	s.globals = make([]*GlobalInstance, maxAddr+1)

	s.globals[nonReleaseTargetAddr] = &GlobalInstance{} // Non-nil!

	// Set up existing function instances.
	var expectedReleasedAddr []globalIndex
	gs := []*GlobalInstance{{index: 1}, {index: 2}, {index: 3}, {index: 4}, {index: maxAddr}}
	for _, g := range gs {
		s.globals[g.index] = &GlobalInstance{} // Non-nil!
		expectedReleasedAddr = append(expectedReleasedAddr, g.index)
	}

	s.releaseGlobalInstances(gs...)

	// Ensure the release targets become nil.
	for _, g := range gs {
		require.Nil(t, s.globals[g.index])
	}

	require.Equal(t, s.releasedGlobalIndex, expectedReleasedAddr)

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
			s.addGlobalInstances(g)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, g.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := globalIndex(10)
		s.releasedGlobalIndex = []globalIndex{1, expectedAddr}
		expectedReleasedGlobalIndex := s.releasedGlobalIndex[:1]

		maxAddr := expectedAddr * 10
		tailInstance := &GlobalInstance{}
		s.globals = make([]*GlobalInstance, maxAddr+1)
		s.globals[maxAddr] = tailInstance

		g := &GlobalInstance{}
		s.addGlobalInstances(g)

		// Index must be reused.
		require.Equal(t, expectedAddr, g.index)

		// And the others must be intact.
		require.Equal(t, tailInstance, s.globals[maxAddr])

		require.Equal(t, expectedReleasedGlobalIndex, s.releasedGlobalIndex)
	})
}

func TestStore_releaseTableInstance(t *testing.T) {
	s := newStore()
	nonReleaseTargetAddr := tableIndex(0)
	maxAddr := tableIndex(10)
	s.tables = make([]*TableInstance, maxAddr+1)

	s.tables[nonReleaseTargetAddr] = &TableInstance{} // Non-nil!

	table := &TableInstance{index: 1}

	s.releaseTableInstance(table)

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
			s.addTableInstance(g)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, g.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := tableIndex(10)
		s.releasedTableIndex = []tableIndex{1000, expectedAddr}
		expectedReleasedTableIndex := s.releasedTableIndex[:1]

		maxAddr := expectedAddr * 10
		tailInstance := &TableInstance{}
		s.tables = make([]*TableInstance, maxAddr+1)
		s.tables[maxAddr] = tailInstance

		table := &TableInstance{}
		s.addTableInstance(table)

		// Index must be reused.
		require.Equal(t, expectedAddr, table.index)

		// And the others must be intact.
		require.Equal(t, tailInstance, s.tables[maxAddr])

		require.Equal(t, expectedReleasedTableIndex, s.releasedTableIndex)
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

	s.releaseMemoryInstance(mem)

	// Ensure the release targets become nil.
	require.Nil(t, s.memories[mem.index])

	require.Equal(t, s.releasedMemoryIndex, []memoryIndex{mem.index})

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
			s.addMemoryInstance(mem)

			// After adding function intance to store, an funcaddr must be assigned.
			require.Equal(t, expectedIndex, mem.index)
		}
	})
	t.Run("reuse released index", func(t *testing.T) {
		s := newStore()
		expectedAddr := memoryIndex(10)
		s.releasedMemoryIndex = []memoryIndex{1000, expectedAddr}
		expectedReleasedMemoryIndex := s.releasedMemoryIndex[:1]

		maxAddr := expectedAddr * 10
		tailInstance := &MemoryInstance{}
		s.memories = make([]*MemoryInstance, maxAddr+1)
		s.memories[maxAddr] = tailInstance

		mem := &MemoryInstance{}
		s.addMemoryInstance(mem)

		// Index must be reused.
		require.Equal(t, expectedAddr, mem.index)

		// And the others must be intact.
		require.Equal(t, tailInstance, s.memories[maxAddr])

		require.Equal(t, expectedReleasedMemoryIndex, s.releasedMemoryIndex)
	})
}

func TestStore_resolveImports(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("module not instantiated", func(t *testing.T) {
		s := newStore()
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: "unknown", Name: "unknown"}}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "module \"unknown\" not instantiated")
	})
	t.Run("export instance not found", func(t *testing.T) {
		s := newStore()
		s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{}, Name: moduleName}
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: "unknown"}}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "\"unknown\" is not exported in module \"test\"")
	})
	t.Run("func", func(t *testing.T) {
		t.Run("unknwon type", func(t *testing.T) {
			s := newStore()
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 100}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "unknown type for function import")
		})
		t.Run("signature mismatch", func(t *testing.T) {
			s := newStore()
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Function: &FunctionInstance{FunctionType: &TypeInstance{Type: &FunctionType{}}},
			}}, Name: moduleName}
			m := &Module{
				TypeSection:   []*FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
			}
			_, _, _, _, _, err := s.resolveImports(m)
			require.Error(t, err)
			require.Contains(t, err.Error(), "signature mimatch: null_f32 != null_null")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			f := &FunctionInstance{FunctionType: &TypeInstance{Type: &FunctionType{Results: []ValueType{ValueTypeF32}}}}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Function: f,
			}}, Name: moduleName}
			m := &Module{
				TypeSection:   []*FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
			}
			functions, _, _, _, moduleImports, err := s.resolveImports(m)
			require.NoError(t, err)
			require.Contains(t, moduleImports, s.moduleInstances[moduleName])
			require.Contains(t, functions, f)
		})
	})
	t.Run("global", func(t *testing.T) {
		t.Run("mutability mismatch", func(t *testing.T) {
			s := newStore()
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{Mutable: false}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{Mutable: true}}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible global import: mutability mismatch")
		})
		t.Run("type mismatch", func(t *testing.T) {
			s := newStore()
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeF64}}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible global import: value type mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			inst := &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {Type: ExternTypeGlobal, Global: inst}}, Name: moduleName}
			_, globals, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: inst.Type}}})
			require.NoError(t, err)
			require.Contains(t, globals, inst)
		})
	})
	t.Run("table", func(t *testing.T) {
		t.Run("element type", func(t *testing.T) {
			s := newStore()
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{ElemType: 0x00}, // Unknown!
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &TableType{ElemType: 0x1}}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible table improt: element type mismatch")
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importTableType := &TableType{Limit: &LimitsType{Min: 2}}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Limit.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible table import: minimum size mismatch")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importTableType := &TableType{Limit: &LimitsType{Max: &max}}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Limit.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible table import: maximum size mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			tableInst := &TableInstance{Max: &max}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: tableInst,
			}}, Name: moduleName}
			_, _, table, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &TableType{Limit: &LimitsType{Max: &max}}}}})
			require.NoError(t, err)
			require.Equal(t, table, tableInst)
		})
	})
	t.Run("memory", func(t *testing.T) {
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importMemoryType := &MemoryType{Min: 2}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{Min: importMemoryType.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible memory import: minimum size mismatch")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importMemoryType := &MemoryType{Max: &max}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.Error(t, err)
			require.Contains(t, err.Error(), "incompatible memory import: maximum size mismatch")
		})
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			memoryInst := &MemoryInstance{Max: &max}
			s.moduleInstances[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: memoryInst,
			}}, Name: moduleName}
			_, _, _, memory, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: &MemoryType{Max: &max}}}})
			require.NoError(t, err)
			require.Equal(t, memory, memoryInst)
		})
	})
}

func TestModuleInstance_validateData(t *testing.T) {
	m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 5)}}
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
	m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 10)}}
	m.applyData([]*DataSegment{
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []byte{0xa, 0xf}},
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []byte{0x1, 0x5}},
	})
	require.Equal(t, []byte{0xa, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x5}, m.MemoryInstance.Buffer)
}

func TestModuleInstance_validateElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		TableInstance: &TableInstance{Table: make([]TableElement, 10)},
		Functions:     make([]*FunctionInstance, 10),
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
		TableInstance: &TableInstance{Table: make([]TableElement, 10)},
		Functions:     make([]*FunctionInstance, 10),
	}
	targetAddr, targetOffset := uint32(1), byte(0)
	targetAddr2, targetOffset2 := functionCounts-1, byte(0x8)
	m.Functions[targetAddr] = &FunctionInstance{FunctionType: &TypeInstance{}, Index: FunctionIndex(targetAddr)}
	m.Functions[targetAddr2] = &FunctionInstance{FunctionType: &TypeInstance{}, Index: FunctionIndex(targetAddr2)}
	m.applyElements([]*ElementSegment{
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset}}, Init: []uint32{uint32(targetAddr)}},
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset2}}, Init: []uint32{targetAddr2}},
	})
	require.Equal(t, FunctionIndex(targetAddr), m.TableInstance.Table[targetOffset].FunctionIndex)
	require.Equal(t, FunctionIndex(targetAddr2), m.TableInstance.Table[targetOffset2].FunctionIndex)
}

func Test_newModuleInstance(t *testing.T) {
	// TODO
}

func TestStore_releaseModuleInstance(t *testing.T) {
	// TODO:
}

func newStore() *Store {
	return NewStore(context.Background(), &catchContext{}, Features20191205)
}
