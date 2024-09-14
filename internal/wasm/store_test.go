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
	"github.com/tetratelabs/wazero/internal/internalapi"
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
				Exports:                 map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory"}},
			},
			expected: true,
		},
		{
			name: "memory exported, one page",
			input: &Module{
				MemorySection:           &Memory{Min: 1, Cap: 1},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				Exports:                 map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory"}},
			},
			expected:    true,
			expectedLen: 65536,
		},
		{
			name: "memory exported, two pages",
			input: &Module{
				MemorySection:           &Memory{Min: 2, Cap: 2},
				MemoryDefinitionSection: []MemoryDefinition{{}},
				Exports:                 map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory"}},
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

func TestStore_Instantiate(t *testing.T) {
	s := newStore()
	m, err := NewHostModule(
		"foo",
		[]string{"fn"},
		map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: func() {}}}},
		api.CoreFeaturesV1,
	)
	require.NoError(t, err)

	sysCtx := sys.DefaultContext(nil)
	mod, err := s.Instantiate(testCtx, m, "bar", sysCtx, []FunctionTypeID{0})
	require.NoError(t, err)
	defer mod.Close(testCtx)

	t.Run("ModuleInstance defaults", func(t *testing.T) {
		require.Equal(t, s.nameToModule["bar"], mod)
		require.Equal(t, s.nameToModule["bar"].MemoryInstance, mod.MemoryInstance)
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
				Exports:                   map[string]*Export{"fn": {Type: ExternTypeFunc, Name: "fn"}},
				FunctionDefinitionSection: []FunctionDefinition{{Functype: &v_v}},
			}, importedModuleName, nil, []FunctionTypeID{0})
			require.NoError(t, err)

			m2, err := s.Instantiate(testCtx, &Module{
				ImportFunctionCount:     1,
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

	m, err := NewHostModule(
		importedModuleName,
		[]string{"fn"},
		map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: func() {}}}},
		api.CoreFeaturesV1,
	)
	require.NoError(t, err)

	s := newStore()
	imported, err := s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
	require.NoError(t, err)

	_, ok := s.nameToModule[imported.Name()]
	require.True(t, ok)

	importingModule := &Module{
		ImportFunctionCount:     1,
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

	// Concurrent instantiate, close should test if locks work on the ns. If they don't, we should see leaked modules
	// after all of these complete, or an error raised.
	P := 8               // max count of goroutines
	N := 1000            // work per goroutine
	if testing.Short() { // Adjust down if `-test.short`
		P = 4
		N = 100
	}
	hammer.NewHammer(t, P, N).Run(func(p, n int) {
		mod, instantiateErr := s.Instantiate(testCtx, importingModule, fmt.Sprintf("%d:%d", p, n), sys.DefaultContext(nil), []FunctionTypeID{0})
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

	m, err := NewHostModule(
		importedModuleName,
		[]string{"fn"},
		map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: func() {}}}},
		api.CoreFeaturesV1,
	)
	require.NoError(t, err)

	s := newStore()
	imported, err := s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
	require.NoError(t, err)

	_, ok := s.nameToModule[imported.Name()]
	require.True(t, ok)

	importingModule := &Module{
		ImportFunctionCount:     1,
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

	const instCount = 10000
	instances := make([]api.Module, instCount)
	for i := 0; i < instCount; i++ {
		mod, instantiateErr := s.Instantiate(testCtx, importingModule, strconv.Itoa(i), sys.DefaultContext(nil), []FunctionTypeID{0})
		require.NoError(t, instantiateErr)
		instances[i] = mod
	}

	hammer.NewHammer(t, 100, 2).Run(func(p, n int) {
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

	m, err := NewHostModule(
		importedModuleName,
		[]string{"fn"},
		map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: func() {}}}},
		api.CoreFeaturesV1,
	)
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

		hm := s.nameToModule[importedModuleName]
		require.NotNil(t, hm)

		_, err = s.Instantiate(testCtx, &Module{
			TypeSection: []FunctionType{v_v},
			ImportSection: []Import{
				// The first import resolve succeeds -> increment hm.dependentCount.
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
				// But the second one tries to import uninitialized-module ->
				{Type: ExternTypeFunc, Module: "non-exist", Name: "fn", DescFunc: 0},
			},
			ImportPerModule: map[string][]*Import{
				importedModuleName: {{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0}},
				"non-exist":        {{Name: "fn", DescFunc: 0}},
			},
		}, importingModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "module[non-exist] not instantiated")
	})

	t.Run("creating engine failed", func(t *testing.T) {
		s := newStore()

		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		hm := s.nameToModule[importedModuleName]
		require.NotNil(t, hm)

		engine := s.Engine.(*mockEngine)
		engine.shouldCompileFail = true

		importingModule := &Module{
			ImportFunctionCount: 1,
			TypeSection:         []FunctionType{v_v},
			FunctionSection:     []uint32{0, 0},
			CodeSection: []Code{
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
			},
			ImportSection: []Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}

		_, err = s.Instantiate(testCtx, importingModule, importingModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "some engine creation error")
	})

	t.Run("start func failed", func(t *testing.T) {
		s := newStore()
		engine := s.Engine.(*mockEngine)
		engine.callFailIndex = 1

		_, err = s.Instantiate(testCtx, m, importedModuleName, nil, []FunctionTypeID{0})
		require.NoError(t, err)

		hm := s.nameToModule[importedModuleName]
		require.NotNil(t, hm)

		startFuncIndex := uint32(1)
		importingModule := &Module{
			ImportFunctionCount: 1,
			TypeSection:         []FunctionType{v_v},
			FunctionSection:     []uint32{0},
			CodeSection:         []Code{{Body: []byte{OpcodeEnd}}},
			StartSection:        &startFuncIndex,
			ImportSection: []Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}

		_, err = s.Instantiate(testCtx, importingModule, importingModuleName, nil, []FunctionTypeID{0})
		require.EqualError(t, err, "start function[1] failed: call failed")
	})
}

type mockEngine struct {
	shouldCompileFail bool
	callFailIndex     int
}

type mockModuleEngine struct {
	name                 string
	callFailIndex        int
	functionRefs         map[Index]Reference
	resolveImportsCalled map[Index]Index
	importedMemModEngine ModuleEngine
	lookupEntries        map[Index]mockModuleEngineLookupEntry
	memoryGrown          int
}

type mockModuleEngineLookupEntry struct {
	m     *ModuleInstance
	index Index
}

type mockCallEngine struct {
	internalapi.WazeroOnlyType
	index         Index
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
func (e *mockModuleEngine) LookupFunction(_ *TableInstance, _ FunctionTypeID, offset Index) (*ModuleInstance, Index) {
	if entry, ok := e.lookupEntries[offset]; ok {
		return entry.m, entry.index
	}
	return nil, 0
}

// CompiledModuleCount implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompiledModuleCount() uint32 { return 0 }

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) DeleteCompiledModule(*Module) {}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *mockEngine) NewModuleEngine(_ *Module, _ *ModuleInstance) (ModuleEngine, error) {
	if e.shouldCompileFail {
		return nil, fmt.Errorf("some engine creation error")
	}
	return &mockModuleEngine{callFailIndex: e.callFailIndex, resolveImportsCalled: map[Index]Index{}}, nil
}

// GetGlobalValue implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) GetGlobalValue(idx Index) (lo, hi uint64) { panic("BUG") }

// SetGlobalValue implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) SetGlobalValue(idx Index, lo, hi uint64) { panic("BUG") }

// OwnsGlobals implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) OwnsGlobals() bool { return false }

// MemoryGrown implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) MemoryGrown() { e.memoryGrown++ }

// DoneInstantiation implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) DoneInstantiation() {}

// FunctionInstanceReference implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) FunctionInstanceReference(i Index) Reference {
	return e.functionRefs[i]
}

// ResolveImportedFunction implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) ResolveImportedFunction(index, _, importedIndex Index, _ ModuleEngine) {
	e.resolveImportsCalled[index] = importedIndex
}

// ResolveImportedMemory implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) ResolveImportedMemory(imp ModuleEngine) {
	e.importedMemModEngine = imp
}

// NewFunction implements the same method as documented on wasm.ModuleEngine.
func (e *mockModuleEngine) NewFunction(index Index) api.Function {
	return &mockCallEngine{index: index, callFailIndex: e.callFailIndex}
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

// Call implements the same method as documented on api.Function.
func (ce *mockCallEngine) Definition() api.FunctionDefinition { return nil }

// Call implements the same method as documented on api.Function.
func (ce *mockCallEngine) Call(ctx context.Context, _ ...uint64) (results []uint64, err error) {
	return nil, ce.CallWithStack(ctx, nil)
}

// CallWithStack implements the same method as documented on api.Function.
func (ce *mockCallEngine) CallWithStack(_ context.Context, _ []uint64) error {
	if ce.callFailIndex >= 0 && ce.index == Index(ce.callFailIndex) {
		return errors.New("call failed")
	}
	return nil
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
		_, err := s.GetFunctionTypeID(&FunctionType{})
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
				actual, err := s.GetFunctionTypeID(&tc)
				require.NoError(t, err)

				expectedTypeID, ok := s.typeIDs[tc.String()]
				require.True(t, ok)
				require.Equal(t, expectedTypeID, actual)
			})
		}
	})
}

func TestGlobalInstance_initialize(t *testing.T) {
	t.Run("basic type const expr", func(t *testing.T) {
		for _, vt := range []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64} {
			t.Run(ValueTypeName(vt), func(t *testing.T) {
				g := &GlobalInstance{Type: GlobalType{ValType: vt}}
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

				g.initialize(nil, expr, nil)

				switch vt {
				case ValueTypeI32:
					require.Equal(t, int32(1), int32(g.Val))
				case ValueTypeI64:
					require.Equal(t, int64(2), int64(g.Val))
				case ValueTypeF32:
					require.Equal(t, float32(math.MaxFloat32), math.Float32frombits(uint32(g.Val)))
				case ValueTypeF64:
					require.Equal(t, math.MaxFloat64, math.Float64frombits(g.Val))
				}
			})
		}
	})
	t.Run("ref.null", func(t *testing.T) {
		tests := []struct {
			name string
			expr *ConstantExpression
		}{
			{
				name: "ref.null (externref)",
				expr: &ConstantExpression{
					Opcode: OpcodeRefNull,
					Data:   []byte{RefTypeExternref},
				},
			},
			{
				name: "ref.null (funcref)",
				expr: &ConstantExpression{
					Opcode: OpcodeRefNull,
					Data:   []byte{RefTypeFuncref},
				},
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				g := GlobalInstance{}
				g.Type.ValType = tc.expr.Data[0]
				g.initialize(nil, tc.expr, nil)
				require.Equal(t, uint64(0), g.Val)
			})
		}
	})
	t.Run("ref.func", func(t *testing.T) {
		g := GlobalInstance{Type: GlobalType{ValType: RefTypeFuncref}}
		g.initialize(nil,
			&ConstantExpression{Opcode: OpcodeRefFunc, Data: []byte{1}},
			func(funcIndex Index) Reference {
				require.Equal(t, Index(1), funcIndex)
				return 0xdeadbeaf
			},
		)
		require.Equal(t, uint64(0xdeadbeaf), g.Val)
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

				g := &GlobalInstance{Type: GlobalType{ValType: tc.valueType}}
				g.initialize(globals, expr, nil)

				switch tc.valueType {
				case ValueTypeI32:
					require.Equal(t, int32(tc.val), int32(g.Val))
				case ValueTypeI64:
					require.Equal(t, int64(tc.val), int64(g.Val))
				case ValueTypeF32:
					require.Equal(t, tc.val, g.Val)
				case ValueTypeF64:
					require.Equal(t, tc.val, g.Val)
				case ValueTypeV128:
					require.Equal(t, uint64(0x1), g.Val)
					require.Equal(t, uint64(0x2), g.ValHi)
				case ValueTypeFuncref, ValueTypeExternref:
					require.Equal(t, tc.val, g.Val)
				}
			})
		}
	})

	t.Run("vector", func(t *testing.T) {
		expr := &ConstantExpression{Data: []byte{
			1, 0, 0, 0, 0, 0, 0, 0,
			2, 0, 0, 0, 0, 0, 0, 0,
		}, Opcode: OpcodeVecV128Const}
		g := GlobalInstance{Type: GlobalType{ValType: ValueTypeV128}}
		g.initialize(nil, expr, nil)
		require.Equal(t, uint64(0x1), g.Val)
		require.Equal(t, uint64(0x2), g.ValHi)
	})
}

func Test_resolveImports(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("module not instantiated", func(t *testing.T) {
		m := &ModuleInstance{s: newStore()}
		err := m.resolveImports(context.Background(), &Module{ImportPerModule: map[string][]*Import{"unknown": {{}}}})
		require.EqualError(t, err, "module[unknown] not instantiated")
	})
	t.Run("export instance not found", func(t *testing.T) {
		m := &ModuleInstance{s: newStore()}
		m.s.nameToModule[moduleName] = &ModuleInstance{Exports: map[string]*Export{}, ModuleName: moduleName}
		err := m.resolveImports(context.Background(), &Module{ImportPerModule: map[string][]*Import{moduleName: {{Name: "unknown"}}}})
		require.EqualError(t, err, "\"unknown\" is not exported in module \"test\"")
	})
	t.Run("func", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				Exports: map[string]*Export{
					name: {Type: ExternTypeFunc, Index: 2},
					"":   {Type: ExternTypeFunc, Index: 4},
				},
				ModuleName: moduleName,
				Source: &Module{
					FunctionSection: []Index{0, 0, 1, 0, 0},
					TypeSection: []FunctionType{
						{Params: []ValueType{ExternTypeFunc}},
						{Params: []ValueType{i32}, Results: []ValueType{ValueTypeV128}},
					},
				},
			}

			module := &Module{
				TypeSection: []FunctionType{
					{Params: []ValueType{i32}, Results: []ValueType{ValueTypeV128}},
					{Params: []ValueType{ExternTypeFunc}},
				},
				ImportFunctionCount: 2,
				ImportPerModule: map[string][]*Import{
					moduleName: {
						{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0, IndexPerType: 0},
						{Module: moduleName, Name: "", Type: ExternTypeFunc, DescFunc: 1, IndexPerType: 1},
					},
				},
			}

			m := &ModuleInstance{Engine: &mockModuleEngine{resolveImportsCalled: map[Index]Index{}}, s: s, Source: module, TypeIDs: []FunctionTypeID{0, 1}}
			err := m.resolveImports(context.Background(), module)
			require.NoError(t, err)

			me := m.Engine.(*mockModuleEngine)
			require.Equal(t, me.resolveImportsCalled[0], Index(2))
			require.Equal(t, me.resolveImportsCalled[1], Index(4))
		})
		t.Run("signature mismatch", func(t *testing.T) {
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				Exports: map[string]*Export{
					name: {Type: ExternTypeFunc, Index: 0},
				},
				ModuleName: moduleName,
				TypeIDs:    []FunctionTypeID{123435},
				Source: &Module{
					FunctionSection: []Index{0},
					TypeSection: []FunctionType{
						{Params: []ValueType{}},
					},
				},
			}
			module := &Module{
				TypeSection: []FunctionType{{Results: []ValueType{ValueTypeF32}}},
				ImportPerModule: map[string][]*Import{
					moduleName: {{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0}},
				},
			}

			m := &ModuleInstance{Engine: &mockModuleEngine{resolveImportsCalled: map[Index]Index{}}, s: s, Source: module}
			err := m.resolveImports(context.Background(), module)
			require.EqualError(t, err, "import func[test.target]: signature mismatch: v_f32 != v_v")
		})
	})
	t.Run("global", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			g := &GlobalInstance{Type: GlobalType{ValType: ValueTypeI32}}
			m := &ModuleInstance{Globals: make([]*GlobalInstance, 1), s: s}
			s.nameToModule[moduleName] = &ModuleInstance{
				Globals: []*GlobalInstance{g},
				Exports: map[string]*Export{name: {Type: ExternTypeGlobal, Index: 0}}, ModuleName: moduleName,
			}
			err := m.resolveImports(
				context.Background(),
				&Module{
					ImportPerModule: map[string][]*Import{moduleName: {{Name: name, Type: ExternTypeGlobal, DescGlobal: g.Type}}},
				},
			)
			require.NoError(t, err)
			require.True(t, globalsContain(m.Globals, g), "expected to find %v in %v", g, m.Globals)
		})
		t.Run("mutability mismatch", func(t *testing.T) {
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				Globals: []*GlobalInstance{{Type: GlobalType{Mutable: false}}},
				Exports: map[string]*Export{name: {
					Type:  ExternTypeGlobal,
					Index: 0,
				}},
				ModuleName: moduleName,
			}
			m := &ModuleInstance{Globals: make([]*GlobalInstance, 1), s: s}
			err := m.resolveImports(
				context.Background(),
				&Module{
					ImportPerModule: map[string][]*Import{moduleName: {
						{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: GlobalType{Mutable: true}},
					}},
				})
			require.EqualError(t, err, "import global[test.target]: mutability mismatch: true != false")
		})
		t.Run("type mismatch", func(t *testing.T) {
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				Globals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}}},
				Exports: map[string]*Export{name: {
					Type:  ExternTypeGlobal,
					Index: 0,
				}},
				ModuleName: moduleName,
			}
			m := &ModuleInstance{Globals: make([]*GlobalInstance, 1), s: s}
			err := m.resolveImports(
				context.Background(),
				&Module{
					ImportPerModule: map[string][]*Import{moduleName: {
						{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeF64}},
					}},
				})
			require.EqualError(t, err, "import global[test.target]: value type mismatch: f64 != i32")
		})
	})
	t.Run("memory", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			max := uint32(10)
			memoryInst := &MemoryInstance{Max: max}
			s := newStore()
			importedME := &mockModuleEngine{}
			s.nameToModule[moduleName] = &ModuleInstance{
				MemoryInstance: memoryInst,
				Exports: map[string]*Export{name: {
					Type: ExternTypeMemory,
				}},
				ModuleName: moduleName,
				Engine:     importedME,
			}
			m := &ModuleInstance{s: s, Engine: &mockModuleEngine{resolveImportsCalled: map[Index]Index{}}}
			err := m.resolveImports(context.Background(), &Module{
				ImportPerModule: map[string][]*Import{
					moduleName: {{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: &Memory{Max: max}}},
				},
			})
			require.NoError(t, err)
			require.Equal(t, m.MemoryInstance, memoryInst)
			require.Equal(t, importedME, m.Engine.(*mockModuleEngine).importedMemModEngine)
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			importMemoryType := &Memory{Min: 2, Cap: 2}
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				MemoryInstance: &MemoryInstance{Min: importMemoryType.Min - 1, Cap: 2},
				Exports: map[string]*Export{name: {
					Type: ExternTypeMemory,
				}},
				ModuleName: moduleName,
			}
			m := &ModuleInstance{s: s}
			err := m.resolveImports(context.Background(), &Module{
				ImportPerModule: map[string][]*Import{
					moduleName: {{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}},
				},
			})
			require.EqualError(t, err, "import memory[test.target]: minimum size mismatch: 2 > 1")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			s.nameToModule[moduleName] = &ModuleInstance{
				MemoryInstance: &MemoryInstance{Max: MemoryLimitPages},
				Exports: map[string]*Export{name: {
					Type: ExternTypeMemory,
				}},
				ModuleName: moduleName,
			}

			max := uint32(10)
			importMemoryType := &Memory{Max: max}
			m := &ModuleInstance{s: s}
			err := m.resolveImports(context.Background(), &Module{
				ImportPerModule: map[string][]*Import{moduleName: {{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}},
			})
			require.EqualError(t, err, "import memory[test.target]: maximum size mismatch: 10 < 65536")
		})
	})
}

func TestModuleInstance_validateData(t *testing.T) {
	m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 5)}}
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
		m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 10)}}
		err := m.applyData([]DataSegment{
			{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: const0}, Init: []byte{0xa, 0xf}},
			{OffsetExpression: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeUint32(8)}, Init: []byte{0x1, 0x5}},
		})
		require.NoError(t, err)
		require.Equal(t, []byte{0xa, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x5}, m.MemoryInstance.Buffer)
		require.Equal(t, [][]byte{{0xa, 0xf}, {0x1, 0x5}}, m.DataInstances)
	})
	t.Run("error", func(t *testing.T) {
		m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 5)}}
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

func TestModuleInstance_applyElements(t *testing.T) {
	leb128_100 := leb128.EncodeInt32(100)

	t.Run("extenref", func(t *testing.T) {
		m := &ModuleInstance{}
		m.Tables = []*TableInstance{{Type: RefTypeExternref, References: make([]Reference, 10)}}
		for i := range m.Tables[0].References {
			m.Tables[0].References[i] = 0xffff // non-null ref.
		}

		// This shouldn't panic.
		m.applyElements([]ElementSegment{{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128_100}}})
		m.applyElements([]ElementSegment{
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: make([]Index, 3)},
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128_100}, Init: make([]Index, 5)}, // Iteration stops at this point, so the offset:5 below shouldn't be applied.
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{5}}, Init: make([]Index, 5)},
		})
		require.Equal(t, []Reference{0, 0, 0, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff},
			m.Tables[0].References)
		m.applyElements([]ElementSegment{
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{5}}, Init: make([]Index, 5)},
		})
		require.Equal(t, []Reference{0, 0, 0, 0xffff, 0xffff, 0, 0, 0, 0, 0}, m.Tables[0].References)
	})
	t.Run("funcref", func(t *testing.T) {
		e := &mockEngine{}
		me, err := e.NewModuleEngine(nil, nil)
		me.(*mockModuleEngine).functionRefs = map[Index]Reference{0: 0xa, 1: 0xaa, 2: 0xaaa, 3: 0xaaaa}
		require.NoError(t, err)
		m := &ModuleInstance{Engine: me, Globals: []*GlobalInstance{{}, {Val: 0xabcde}}}

		m.Tables = []*TableInstance{{Type: RefTypeFuncref, References: make([]Reference, 10)}}
		for i := range m.Tables[0].References {
			m.Tables[0].References[i] = 0xffff // non-null ref.
		}

		// This shouldn't panic.
		m.applyElements([]ElementSegment{{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128_100}, Init: []Index{1, 2, 3}}})
		m.applyElements([]ElementSegment{
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0, 1, 2}},
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{9}}, Init: []Index{1 | elementInitImportedGlobalReferenceType}},
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128_100}, Init: make([]Index, 5)}, // Iteration stops at this point, so the offset:5 below shouldn't be applied.
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{5}}, Init: make([]Index, 5)},
		})
		require.Equal(t, []Reference{0xa, 0xaa, 0xaaa, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xffff, 0xabcde},
			m.Tables[0].References)
		m.applyElements([]ElementSegment{
			{Mode: ElementModeActive, OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{5}}, Init: []Index{0, ElementInitNullReference, 2}},
		})
		require.Equal(t, []Reference{0xa, 0xaa, 0xaaa, 0xffff, 0xffff, 0xa, 0xffff, 0xaaa, 0xffff, 0xabcde},
			m.Tables[0].References)
	})
}
