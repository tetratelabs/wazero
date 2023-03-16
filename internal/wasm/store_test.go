package wasm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
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
			name: "memory not exported, one page",
			input: &Module{
				MemorySection:           &Memory{Min: 1, Cap: 1},
				MemoryDefinitionSection: []MemoryDefinition{{}},
			},
		},
		{
			name: "memory exported, different name",
			input: &Module{
				MemorySection:           &Memory{Min: 1, Cap: 1},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				ExportSection:           []Export{{Type: ExternTypeMemory, Name: "momory", Index: 0}},
			},
		},
		{
			name: "memory exported, but zero length",
			input: &Module{
				MemorySection:           &Memory{},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				ExportSection:           []Export{{Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected: true,
		},
		{
			name: "memory exported, one page",
			input: &Module{
				MemorySection:           &Memory{Min: 1, Cap: 1},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				ExportSection:           []Export{{Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536,
		},
		{
			name: "memory exported, two pages",
			input: &Module{
				MemorySection:           &Memory{Min: 2, Cap: 2},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				ExportSection:           []Export{{Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536 * 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			s := newStore()

			instance, err := s.Instantiate(testCtx, tc.input, "test", nil, nil)
			require.NoError(t, err)

			mem := instance.ExportedMemory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

func TestNewStore(t *testing.T) {
	s := NewStore(api.CoreFeaturesV1, &mockEngine{shouldCompileFail: false, callFailIndex: -1})
	// Ensures that a newly created store has the pre allocated type IDs.
	for k, v := range preAllocatedTypeIDs {
		actual, ok := s.typeIDs[k]
		require.True(t, ok)
		require.Equal(t, v, actual)
	}
}

func TestStore_Instantiate(t *testing.T) {
	s := newStore()
	m, err := NewHostModule("foo", map[string]interface{}{"fn": func() {}}, map[string]*HostFuncNames{"fn": {}}, api.CoreFeaturesV1)
	require.NoError(t, err)

	sysCtx := sys.DefaultContext(nil)
	mod, err := s.Instantiate(testCtx, m, "bar", sysCtx, []FunctionTypeID{0})
	require.NoError(t, err)
	defer mod.Close(testCtx)

	t.Run("CallContext defaults", func(t *testing.T) {
		require.Equal(t, s.nameToNode["bar"].module, mod.module)
		require.Equal(t, s.nameToNode["bar"].module.Memory, mod.memory)
		require.Equal(t, s, mod.s)
		require.Equal(t, sysCtx, mod.Sys)
	})
}

func TestStore_CloseWithExitCode(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	tests := []struct {
		name       string
		testClosed bool
	}{
		{
			name:       "nothing closed",
			testClosed: false,
		},
		{
			name:       "partially closed",
			testClosed: true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			s := newStore()

			_, err := s.Instantiate(testCtx, &Module{
				TypeSection:               []FunctionType{v_v},
				FunctionSection:           []uint32{0},
				CodeSection:               []Code{{Body: []byte{OpcodeEnd}}},
				ExportSection:             []Export{{Type: ExternTypeFunc, Index: 0, Name: "fn"}},
				FunctionDefinitionSection: []FunctionDefinition{{funcType: &v_v}},
			}, importedModuleName, nil, []FunctionTypeID{0})
			require.NoError(t, err)

			m2, err := s.Instantiate(testCtx, &Module{
				TypeSection:             []FunctionType{v_v},
				ImportSection:           []Import{{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0}},
				MemorySection:           &Memory{Min: 1, Cap: 1},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				GlobalSection:           []Global{{Type: GlobalType{}, Init: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}}},
				TableSection:            []Table{{Min: 10}},
			}, importingModuleName, nil, []FunctionTypeID{0})
			require.NoError(t, err)

			if tc.testClosed {
				err = m2.CloseWithExitCode(testCtx, 2)
				require.NoError(t, err)
			}

			err = s.CloseWithExitCode(testCtx, 2)
			require.NoError(t, err)

			// If Store.CloseWithExitCode was dispatched properly, modules should be empty
			require.Nil(t, s.moduleList)

			// Store state zeroed
			require.Zero(t, len(s.typeIDs))
		})
	}
}

func TestStore_hammer(t *testing.T) {
	const importedModuleName = "imported"

	m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func() {}}, map[string]*HostFuncNames{"fn": {}}, api.CoreFeaturesV1)
	require.NoError(t, err)

	s := newStore()
	imported, err := s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
	require.NoError(t, err)

	_, ok := s.nameToNode[imported.Name()]
	require.True(t, ok)

	importingModule := &Module{
		TypeSection:             []FunctionType{v_v},
		FunctionSection:         []uint32{0},
		CodeSection:             []Code{{Body: []byte{OpcodeEnd}}},
		MemorySection:           &Memory{Min: 1, Cap: 1},
		MemoryDefinitionSection: []MemoryDefinition{{}},
		GlobalSection: []Global{{
			Type: GlobalType{ValType: ValueTypeI32},
			Init: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(1)},
		}},
		TableSection: []Table{{Min: 10}},
		ImportSection: []Import{
			{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
		},
	}
	importingModule.BuildFunctionDefinitions()

	// Concurrent instantiate, close should test if locks work on the ns. If they don't, we should see leaked modules
	// after all of these complete, or an error raised.
	P := 8               // max count of goroutines
	N := 1000            // work per goroutine
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
		N = 100
	}
	hammer.NewHammer(t, P, N).Run(func(name string) {
		mod, instantiateErr := s.Instantiate(testCtx, importingModule, name, sys.DefaultContext(nil), []FunctionTypeID{0})
		require.NoError(t, instantiateErr)
		require.NoError(t, mod.Close(testCtx))
	}, nil)
	if t.Failed() {
		return // At least one test failed, so return now.
	}

	// Close the imported module.
	require.NoError(t, imported.Close(testCtx))

	// All instances are freed.
	require.Nil(t, s.moduleList)
}

func TestStore_hammer_close(t *testing.T) {
	const importedModuleName = "imported"

	m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func() {}}, map[string]*HostFuncNames{"fn": {}}, api.CoreFeaturesV1)
	require.NoError(t, err)

	s := newStore()
	imported, err := s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
	require.NoError(t, err)

	_, ok := s.nameToNode[imported.Name()]
	require.True(t, ok)

	importingModule := &Module{
		TypeSection:             []FunctionType{v_v},
		FunctionSection:         []uint32{0},
		CodeSection:             []Code{{Body: []byte{OpcodeEnd}}},
		MemorySection:           &Memory{Min: 1, Cap: 1},
		MemoryDefinitionSection: []MemoryDefinition{{}},
		GlobalSection: []Global{{
			Type: GlobalType{ValType: ValueTypeI32},
			Init: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(1)},
		}},
		TableSection: []Table{{Min: 10}},
		ImportSection: []Import{
			{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
		},
	}
	importingModule.BuildFunctionDefinitions()

	const instCount = 10000
	instances := make([]api.Module, instCount)
	for i := 0; i < instCount; i++ {
		mod, instantiateErr := s.Instantiate(testCtx, importingModule, strconv.Itoa(i), sys.DefaultContext(nil), []FunctionTypeID{0})
		require.NoError(t, instantiateErr)
		instances[i] = mod
	}

	hammer.NewHammer(t, 100, 2).Run(func(name string) {
		for i := 0; i < instCount; i++ {
			if i == instCount/2 {
				// Close store concurrently as well.
				err := s.CloseWithExitCode(testCtx, 0)
				require.NoError(t, err)
			}
			err := instances[i].CloseWithExitCode(testCtx, 0)
			require.NoError(t, err)
		}
		require.NoError(t, err)
	}, nil)
	if t.Failed() {
		return // At least one test failed, so return now.
	}

	// All instances are freed.
	require.Nil(t, s.moduleList)
}

func TestStore_Instantiate_Errors(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func() {}}, map[string]*HostFuncNames{"fn": {}}, api.CoreFeaturesV1)
	require.NoError(t, err)

	t.Run("Fails if module name already in use", func(t *testing.T) {
		s := newStore()
		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		// Trying to register it again should fail
		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "module[imported] has already been instantiated")
	})

	t.Run("fail resolve import", func(t *testing.T) {
		s := newStore()
		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		hm := s.nameToNode[importedModuleName]
		require.NotNil(t, hm)

		_, err = s.Instantiate(testCtx, &Module{
			TypeSection: []FunctionType{v_v},
			ImportSection: []Import{
				// The first import resolve succeeds -> increment hm.dependentCount.
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
				// But the second one tries to import uninitialized-module ->
				{Type: ExternTypeFunc, Module: "non-exist", Name: "fn", DescFunc: 0},
			},
		}, importingModuleName, nil, nil)
		require.EqualError(t, err, "module[non-exist] not instantiated")
	})

	t.Run("creating engine failed", func(t *testing.T) {
		s := newStore()

		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		hm := s.nameToNode[importedModuleName]
		require.NotNil(t, hm)

		engine := s.Engine.(*mockEngine)
		engine.shouldCompileFail = true

		importingModule := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []uint32{0, 0},
			CodeSection: []Code{
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
			},
			ImportSection: []Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}
		importingModule.BuildFunctionDefinitions()

		_, err = s.Instantiate(testCtx, importingModule, importingModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "some engine creation error")
	})

	t.Run("start func failed", func(t *testing.T) {
		s := newStore()
		engine := s.Engine.(*mockEngine)
		engine.callFailIndex = 1

		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		hm := s.nameToNode[importedModuleName]
		require.NotNil(t, hm)

		startFuncIndex := uint32(1)
		importingModule := &Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []uint32{0},
			CodeSection:     []Code{{Body: []byte{OpcodeEnd}}},
			StartSection:    &startFuncIndex,
			ImportSection: []Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}
		importingModule.BuildFunctionDefinitions()

		_, err = s.Instantiate(testCtx, importingModule, importingModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "start function[1] failed: call failed")
	})
}

type mockEngine struct {
	shouldCompileFail bool
	callFailIndex     int
}

type mockModuleEngine struct {
	name          string
	callFailIndex int
	functionRefs  map[Index]Reference
}

type mockCallEngine struct {
	f             *FunctionInstance
	callFailIndex int
}

func newStore() *Store {
	return NewStore(api.CoreFeaturesV1, &mockEngine{shouldCompileFail: false, callFailIndex: -1})
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) Close() error {
	return nil
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompileModule(context.Context, *Module, []experimental.FunctionListener, bool) error {
	return nil
}

// LookupFunction implements the same method as documented on wasm.Engine.
func (e *mockModuleEngine) LookupFunction(*TableInstance, FunctionTypeID, Index) (Index, error) {
	return 0, nil
}

// CompiledModuleCount implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompiledModuleCount() uint32 { return 0 }

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) DeleteCompiledModule(*Module) {}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *mockEngine) NewModuleEngine(_ string, _ *Module, _ []FunctionInstance) (ModuleEngine, error) {
	if e.shouldCompileFail {
		return nil, fmt.Errorf("some engine creation error")
	}
	return &mockModuleEngine{callFailIndex: e.callFailIndex}, nil
}

// FunctionInstanceReference implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) FunctionInstanceReference(i Index) Reference {
	return e.functionRefs[i]
}

// NewCallEngine implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) NewCallEngine(callCtx *CallContext, f *FunctionInstance) (CallEngine, error) {
	return &mockCallEngine{f: f, callFailIndex: e.callFailIndex}, nil
}

// InitializeFuncrefGlobals implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) InitializeFuncrefGlobals(globals []*GlobalInstance) {}

// Name implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) Name() string {
	return e.name
}

// Close implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) Close(context.Context) {
}

// Call implements the same method as documented on wasm.ModuleEngine.
func (ce *mockCallEngine) Call(ctx context.Context, callCtx *CallContext, _ []uint64) (results []uint64, err error) {
	if ce.callFailIndex >= 0 && ce.f.Definition.Index() == Index(ce.callFailIndex) {
		err = errors.New("call failed")
		return
	}
	return
}

func TestStore_getFunctionTypeID(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := newStore()
		const max = 10
		s.functionMaxTypes = max
		s.typeIDs = make(map[string]FunctionTypeID)
		for i := 0; i < max; i++ {
			s.typeIDs[strconv.Itoa(i)] = 0
		}
		_, err := s.getFunctionTypeID(&FunctionType{})
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		tests := []FunctionType{
			{Params: []ValueType{}},
			{Params: []ValueType{ValueTypeF32}},
			{Results: []ValueType{ValueTypeF64}},
			{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.String(), func(t *testing.T) {
				s := newStore()
				actual, err := s.getFunctionTypeID(&tc)
				require.NoError(t, err)

				expectedTypeID, ok := s.typeIDs[tc.String()]
				require.True(t, ok)
				require.Equal(t, expectedTypeID, actual)
			})
		}
	})
}

func TestExecuteConstExpression(t *testing.T) {
	t.Run("basic type const expr", func(t *testing.T) {
		for _, vt := range []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64} {
			t.Run(ValueTypeName(vt), func(t *testing.T) {
				expr := &ConstantExpression{}
				switch vt {
				case ValueTypeI32:
					expr.Data = []byte{1}
					expr.Opcode = OpcodeI32Const
				case ValueTypeI64:
					expr.Data = []byte{2}
					expr.Opcode = OpcodeI64Const
				case ValueTypeF32:
					expr.Data = u64.LeBytes(api.EncodeF32(math.MaxFloat32))
					expr.Opcode = OpcodeF32Const
				case ValueTypeF64:
					expr.Data = u64.LeBytes(api.EncodeF64(math.MaxFloat64))
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
	t.Run("reference types", func(t *testing.T) {
		tests := []struct {
			name string
			expr *ConstantExpression
			exp  interface{}
		}{
			{
				name: "ref.null (externref)",
				expr: &ConstantExpression{
					Opcode: OpcodeRefNull,
					Data:   []byte{RefTypeExternref},
				},
				exp: int64(0),
			},
			{
				name: "ref.null (funcref)",
				expr: &ConstantExpression{
					Opcode: OpcodeRefNull,
					Data:   []byte{RefTypeFuncref},
				},
				exp: int64(0),
			},
			{
				name: "ref.func",
				expr: &ConstantExpression{
					Opcode: OpcodeRefFunc,
					Data:   []byte{1},
				},
				exp: uint32(1),
			},
			{
				name: "ref.func",
				expr: &ConstantExpression{
					Opcode: OpcodeRefFunc,
					Data:   []byte{0x5d},
				},
				exp: uint32(93),
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				val := executeConstExpression(nil, tc.expr)
				require.Equal(t, tc.exp, val)
			})
		}
	})
	t.Run("global expr", func(t *testing.T) {
		tests := []struct {
			valueType  ValueType
			val, valHi uint64
		}{
			{valueType: ValueTypeI32, val: 10},
			{valueType: ValueTypeI64, val: 20},
			{valueType: ValueTypeF32, val: uint64(math.Float32bits(634634432.12311))},
			{valueType: ValueTypeF64, val: math.Float64bits(1.12312311)},
			{valueType: ValueTypeV128, val: 0x1, valHi: 0x2},
			{valueType: ValueTypeExternref, val: 0x12345},
			{valueType: ValueTypeFuncref, val: 0x54321},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(ValueTypeName(tc.valueType), func(t *testing.T) {
				// The index specified in Data equals zero.
				expr := &ConstantExpression{Data: []byte{0}, Opcode: OpcodeGlobalGet}
				globals := []*GlobalInstance{{Val: tc.val, ValHi: tc.valHi, Type: GlobalType{ValType: tc.valueType}}}

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
					require.Equal(t, api.DecodeF32(tc.val), actual)
				case ValueTypeF64:
					actual, ok := val.(float64)
					require.True(t, ok)
					require.Equal(t, api.DecodeF64(tc.val), actual)
				case ValueTypeV128:
					vector, ok := val.([2]uint64)
					require.True(t, ok)
					require.Equal(t, uint64(0x1), vector[0])
					require.Equal(t, uint64(0x2), vector[1])
				case ValueTypeFuncref, ValueTypeExternref:
					actual, ok := val.(int64)
					require.True(t, ok)
					require.Equal(t, int64(tc.val), actual)
				}
			})
		}
	})

	t.Run("vector", func(t *testing.T) {
		expr := &ConstantExpression{Data: []byte{
			1, 0, 0, 0, 0, 0, 0, 0,
			2, 0, 0, 0, 0, 0, 0, 0,
		}, Opcode: OpcodeVecV128Const}
		val := executeConstExpression(nil, expr)
		require.NotNil(t, val)
		vector, ok := val.([2]uint64)
		require.True(t, ok)
		require.Equal(t, uint64(0x1), vector[0])
		require.Equal(t, uint64(0x2), vector[1])
	})
}

func Test_resolveImports(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("module not instantiated", func(t *testing.T) {
		modules := map[string]*ModuleInstance{}
		_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: "unknown", Name: "unknown"}}}, modules)
		require.EqualError(t, err, "module[unknown] not instantiated")
	})
	t.Run("export instance not found", func(t *testing.T) {
		modules := map[string]*ModuleInstance{
			moduleName: {Exports: map[string]ExportInstance{}, Name: moduleName},
		}
		_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: "unknown"}}}, modules)
		require.EqualError(t, err, "\"unknown\" is not exported in module \"test\"")
	})
	t.Run("func", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			externMod := &ModuleInstance{
				Functions: []FunctionInstance{
					{Definition: &FunctionDefinition{funcType: &FunctionType{Results: []ValueType{ValueTypeF32}}}},
					{Definition: &FunctionDefinition{funcType: &FunctionType{Results: []ValueType{ValueTypeI32}}}},
				},
				Exports: map[string]ExportInstance{
					name: {Type: ExternTypeFunc, Index: 0},
					"":   {Type: ExternTypeFunc, Index: 1},
				},
				Name: moduleName,
			}
			modules := map[string]*ModuleInstance{
				moduleName: externMod,
			}
			m := &Module{
				TypeSection: []FunctionType{{Results: []ValueType{ValueTypeF32}}, {Results: []ValueType{ValueTypeI32}}},
				ImportSection: []Import{
					{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0},
					{Module: moduleName, Name: "", Type: ExternTypeFunc, DescFunc: 1},
				},
			}
			functions, _, _, _, err := resolveImports(m, modules)
			require.NoError(t, err)
			require.True(t, functionsContain(functions, &externMod.Functions[0]), "expected to find %v in %v", &externMod.Functions[0], functions)
			require.True(t, functionsContain(functions, &externMod.Functions[1]), "expected to find %v in %v", &externMod.Functions[1], functions)
		})
		t.Run("type out of range", func(t *testing.T) {
			modules := map[string]*ModuleInstance{
				moduleName: {Exports: map[string]ExportInstance{name: {}}, Name: moduleName},
			}
			_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 100}}}, modules)
			require.EqualError(t, err, "import[0] func[test.target]: function type out of range")
		})
		t.Run("signature mismatch", func(t *testing.T) {
			externMod := &ModuleInstance{
				Functions: []FunctionInstance{{Definition: &FunctionDefinition{funcType: &FunctionType{}}}},
				Exports: map[string]ExportInstance{
					name: {Type: ExternTypeFunc, Index: 0},
				},
				Name: moduleName,
			}
			modules := map[string]*ModuleInstance{moduleName: externMod}
			m := &Module{
				TypeSection:   []FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
			}
			_, _, _, _, err := resolveImports(m, modules)
			require.EqualError(t, err, "import[0] func[test.target]: signature mismatch: v_f32 != v_v")
		})
	})
	t.Run("global", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			g := &GlobalInstance{Type: GlobalType{ValType: ValueTypeI32}}
			modules := map[string]*ModuleInstance{
				moduleName: {
					Globals: []*GlobalInstance{g},
					Exports: map[string]ExportInstance{name: {Type: ExternTypeGlobal, Index: 0}}, Name: moduleName,
				},
			}
			_, globals, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: g.Type}}}, modules)
			require.NoError(t, err)
			require.True(t, globalsContain(globals, g), "expected to find %v in %v", g, globals)
		})
		t.Run("mutability mismatch", func(t *testing.T) {
			modules := map[string]*ModuleInstance{
				moduleName: {
					Globals: []*GlobalInstance{{Type: GlobalType{Mutable: false}}},
					Exports: map[string]ExportInstance{name: {
						Type:  ExternTypeGlobal,
						Index: 0,
					}},
					Name: moduleName,
				},
			}
			_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: GlobalType{Mutable: true}}}}, modules)
			require.EqualError(t, err, "import[0] global[test.target]: mutability mismatch: true != false")
		})
		t.Run("type mismatch", func(t *testing.T) {
			modules := map[string]*ModuleInstance{
				moduleName: {
					Globals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}}},
					Exports: map[string]ExportInstance{name: {
						Type:  ExternTypeGlobal,
						Index: 0,
					}},
					Name: moduleName,
				},
			}
			_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeF64}}}}, modules)
			require.EqualError(t, err, "import[0] global[test.target]: value type mismatch: f64 != i32")
		})
	})
	t.Run("memory", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			max := uint32(10)
			memoryInst := &MemoryInstance{Max: max}
			modules := map[string]*ModuleInstance{
				moduleName: {
					Memory: memoryInst,
					Exports: map[string]ExportInstance{name: {
						Type: ExternTypeMemory,
					}},
					Name: moduleName,
				},
			}
			_, _, _, memory, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: &Memory{Max: max}}}}, modules)
			require.NoError(t, err)
			require.Equal(t, memory, memoryInst)
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			importMemoryType := &Memory{Min: 2, Cap: 2}
			modules := map[string]*ModuleInstance{
				moduleName: {
					Memory: &MemoryInstance{Min: importMemoryType.Min - 1, Cap: 2},
					Exports: map[string]ExportInstance{name: {
						Type: ExternTypeMemory,
					}},
					Name: moduleName,
				},
			}
			_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}}, modules)
			require.EqualError(t, err, "import[0] memory[test.target]: minimum size mismatch: 2 > 1")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			max := uint32(10)
			importMemoryType := &Memory{Max: max}
			modules := map[string]*ModuleInstance{
				moduleName: {
					Memory: &MemoryInstance{Max: MemoryLimitPages},
					Exports: map[string]ExportInstance{name: {
						Type: ExternTypeMemory,
					}},
					Name: moduleName,
				},
			}
			_, _, _, _, err := resolveImports(&Module{ImportSection: []Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}}, modules)
			require.EqualError(t, err, "import[0] memory[test.target]: maximum size mismatch: 10 < 65536")
		})
	})
}

func TestModuleInstance_validateData(t *testing.T) {
	m := &ModuleInstance{Memory: &MemoryInstance{Buffer: make([]byte, 5)}}
	tests := []struct {
		name   string
		data   []DataSegment
		expErr string
	}{
		{
			name: "ok",
			data: []DataSegment{
				{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []byte{0}},
				{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)}, Init: []byte{0}},
			},
		},
		{
			name: "out of bounds - single one byte",
			data: []DataSegment{
				{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(5)}, Init: []byte{0}},
			},
			expErr: "data[0]: out of bounds memory access",
		},
		{
			name: "out of bounds - multi bytes",
			data: []DataSegment{
				{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(0)}, Init: []byte{0}},
				{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(3)}, Init: []byte{0, 1, 2}},
			},
			expErr: "data[1]: out of bounds memory access",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateData(tc.data)
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModuleInstance_applyData(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := &ModuleInstance{Memory: &MemoryInstance{Buffer: make([]byte, 10)}}
		err := m.applyData([]DataSegment{
			{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: const0}, Init: []byte{0xa, 0xf}},
			{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeUint32(8)}, Init: []byte{0x1, 0x5}},
		})
		require.NoError(t, err)
		require.Equal(t, []byte{0xa, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x5}, m.Memory.Buffer)
		require.Equal(t, [][]byte{{0xa, 0xf}, {0x1, 0x5}}, m.DataInstances)
	})
	t.Run("error", func(t *testing.T) {
		m := &ModuleInstance{Memory: &MemoryInstance{Buffer: make([]byte, 5)}}
		err := m.applyData([]DataSegment{
			{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeUint32(8)}, Init: []byte{}},
		})
		require.EqualError(t, err, "data[0]: out of bounds memory access")
	})
}

func globalsContain(globals []*GlobalInstance, want *GlobalInstance) bool {
	for _, f := range globals {
		if f == want {
			return true
		}
	}
	return false
}

func functionsContain(functions []*FunctionInstance, want *FunctionInstance) bool {
	for _, f := range functions {
		if f == want {
			return true
		}
	}
	return false
}

func TestModuleInstance_applyTableInits(t *testing.T) {
	t.Run("extenref", func(t *testing.T) {
		tables := []*TableInstance{{Type: RefTypeExternref, References: make([]Reference, 10)}}
		for i := range tables[0].References {
			tables[0].References[i] = 0xffff // non-null ref.
		}
		m := &ModuleInstance{}

		// This shouldn't panic.
		m.applyTableInits(tables, []tableInitEntry{{offset: 100}})
		m.applyTableInits(tables, []tableInitEntry{
			{offset: 0, nullExternRefCount: 3},
			{offset: 100}, // Iteration stops at this point, so the offset:5 below shouldn't be applied.
			{offset: 5, nullExternRefCount: 5},
		})
		require.Equal(t, []Reference{0, 0, 0, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff},
			tables[0].References)
		m.applyTableInits(tables, []tableInitEntry{
			{offset: 5, nullExternRefCount: 5},
		})
		require.Equal(t, []Reference{0, 0, 0, 0xffff, 0xffff, 0, 0, 0, 0, 0}, tables[0].References)
	})
	t.Run("funcref", func(t *testing.T) {
		e := &mockEngine{}
		me, err := e.NewModuleEngine("", nil, nil)
		me.(*mockModuleEngine).functionRefs = map[Index]Reference{0: 0xa, 1: 0xaa, 2: 0xaaa, 3: 0xaaaa}
		require.NoError(t, err)
		m := &ModuleInstance{Engine: me}

		tables := []*TableInstance{{Type: RefTypeFuncref, References: make([]Reference, 10)}}
		for i := range tables[0].References {
			tables[0].References[i] = 0xffff // non-null ref.
		}

		// This shouldn't panic.
		m.applyTableInits(tables, []tableInitEntry{{offset: 100}})
		m.applyTableInits(tables, []tableInitEntry{
			{offset: 0, functionIndexes: []*Index{uint32Ptr(0), uint32Ptr(1), uint32Ptr(2)}},
			{offset: 100}, // Iteration stops at this point, so the offset:5 below shouldn't be applied.
			{offset: 5, nullExternRefCount: 5},
		})
		require.Equal(t, []Reference{0xa, 0xaa, 0xaaa, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff},
			tables[0].References)
		m.applyTableInits(tables, []tableInitEntry{
			{offset: 5, functionIndexes: []*Index{uint32Ptr(0), nil, uint32Ptr(2)}},
		})
		require.Equal(t, []Reference{0xa, 0xaa, 0xaaa, 0xffff, 0xffff, 0xa, 0xffff, 0xaaa, 0xffff, 0xffff},
			tables[0].References)
	})
}

// TestPreAllocatedTypeIDs ensures that PreAllocatedTypeIDs has no duplication on the values (FunctionTypeID).
func TestPreAllocatedTypeIDs(t *testing.T) {
	exists := make(map[FunctionTypeID]struct{}, len(preAllocatedTypeIDs))
	for _, v := range preAllocatedTypeIDs {
		_, ok := exists[v]
		require.False(t, ok)
		exists[v] = struct{}{}
	}
}
