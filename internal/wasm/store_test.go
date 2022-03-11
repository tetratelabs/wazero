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
			input: &Module{MemorySection: &Memory{Min: 1}},
		},
		{
			name:  "memory not exported, one page",
			input: &Module{MemorySection: &Memory{Min: 1}},
		},
		{
			name: "memory exported, different name",
			input: &Module{
				MemorySection: &Memory{Min: 1},
				ExportSection: map[string]*Export{"momory": {Type: ExternTypeMemory, Name: "momory", Index: 0}},
			},
		},
		{
			name: "memory exported, but zero length",
			input: &Module{
				MemorySection: &Memory{},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected: true,
		},
		{
			name: "memory exported, one page",
			input: &Module{
				MemorySection: &Memory{Min: 1},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536,
		},
		{
			name: "memory exported, two pages",
			input: &Module{
				MemorySection: &Memory{Min: 2},
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

			mem := instance.ExportedMemory("memory")
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
	require.Equal(t, "Module[module]", s.Module(m.module.Name).String())
}

func TestStore_CloseModule(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	for _, tc := range []struct {
		name        string
		initializer func(t *testing.T, s *Store)
	}{
		{
			name: "Module imports HostModule",
			initializer: func(t *testing.T, s *Store) {
				m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.Module) {}})
				require.NoError(t, err)
				_, err = s.Instantiate(m, importedModuleName)
				require.NoError(t, err)
			},
		},
		{
			name: "Module imports Module",
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
				MemorySection: &Memory{Min: 1},
				GlobalSection: []*Global{{Type: &GlobalType{}, Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}}},
				TableSection:  &Table{Min: 10},
			}, importingModuleName)
			require.NoError(t, err)

			importedMod, ok := s.modules[importedModuleName]
			require.True(t, ok)

			importingMod, ok := s.modules[importingModuleName]
			require.True(t, ok)

			// We shouldn't be able to release the imported module as it is in use!
			require.Error(t, s.CloseModule(importedModuleName))

			// Make sure modules are not closed yet.
			require.False(t, importingMod.closed)
			require.False(t, importedMod.closed)

			// Can release the importing module
			require.NoError(t, s.CloseModule(importingModuleName))
			require.NotContains(t, s.modules, importingMod.Name)
			require.True(t, importingMod.closed) // only importing one is closed.
			require.False(t, importedMod.closed)

			// Can re-release the importing module
			require.NoError(t, s.CloseModule(importingModuleName))
			require.True(t, importingMod.closed)
			require.False(t, importedMod.closed)

			// Now we should be able to release the imported module.
			require.NoError(t, s.CloseModule(importedModuleName))
			require.Nil(t, s.modules[importedModuleName])
			require.NotContains(t, s.modules, importedModuleName)
			require.True(t, importingMod.closed)
			require.True(t, importedMod.closed)
		})
	}
}

func TestStore_concurrent(t *testing.T) {
	const importedModuleName = "imported"
	const goroutines = 1000

	m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.Module) {}})
	require.NoError(t, err)

	var wg sync.WaitGroup

	s := newStore()
	_, err = s.Instantiate(m, importedModuleName)
	require.NoError(t, err)

	hm, ok := s.modules[importedModuleName]
	require.True(t, ok)

	importingModule := &Module{
		TypeSection:     []*FunctionType{{}},
		FunctionSection: []uint32{0},
		CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
		MemorySection:   &Memory{Min: 1},
		GlobalSection:   []*Global{{Type: &GlobalType{}, Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}}},
		TableSection:    &Table{Min: 10},
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
				err := s.CloseModule(importedModuleName)
				require.Error(t, err)
			}
		}(i)

		// TODO: in addition to the normal instantiation, we should try to instantiate host module in conjunction.
		// after making store.NewHostModule goroutine-safe.
	}
	wg.Wait()

	// At this point 1000 modules import host modules.
	require.Equal(t, goroutines, hm.dependentCount)

	// Concurrent release.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			err := s.CloseModule(strconv.Itoa(i))
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Now all the importing instances were released, the imported module can be freed.
	require.Zero(t, hm.dependentCount)
	require.NoError(t, s.CloseModule(hm.Name))

	// All instances are freed.
	require.Len(t, s.modules, 0)
}

func TestStore_Instantiate_Errors(t *testing.T) {
	const importedModuleName = "imported"
	const importingModuleName = "test"

	m, err := NewHostModule(importedModuleName, map[string]interface{}{"fn": func(wasm.Module) {}})
	require.NoError(t, err)

	t.Run("Fails if module name already in use", func(t *testing.T) {
		s := newStore()
		_, err = s.Instantiate(m, importedModuleName)
		require.NoError(t, err)

		// Trying to register it again should fail
		_, err = s.Instantiate(m, importedModuleName)
		require.EqualError(t, err, "module imported has already been instantiated")
	})

	t.Run("fail resolve import", func(t *testing.T) {
		s := newStore()
		_, err = s.Instantiate(m, importedModuleName)
		require.NoError(t, err)

		hm := s.modules[importedModuleName]
		require.NotNil(t, hm)

		_, err = s.Instantiate(&Module{
			TypeSection: []*FunctionType{{}},
			ImportSection: []*Import{
				// The first import resolve succeeds -> increment hm.dependentCount.
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
				// But the second one tries to import uninitialized-module ->
				{Type: ExternTypeFunc, Module: "non-exist", Name: "fn", DescFunc: 0},
			},
		}, importingModuleName)
		require.EqualError(t, err, "module[non-exist] not instantiated")

		// hm.dependentCount must be intact as the instantiation failed.
		require.Zero(t, hm.dependentCount)
	})

	t.Run("compilation failed", func(t *testing.T) {
		s := newStore()

		_, err = s.Instantiate(m, importedModuleName)
		require.NoError(t, err)

		hm := s.modules[importedModuleName]
		require.NotNil(t, hm)

		engine := s.engine.(*mockEngine)
		engine.shouldCompileFail = true

		_, err = s.Instantiate(&Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []uint32{0, 0},
			CodeSection: []*Code{
				{Body: []byte{OpcodeEnd}},
				{Body: []byte{OpcodeEnd}},
			},
			ImportSection: []*Import{
				{Type: ExternTypeFunc, Module: importedModuleName, Name: "fn", DescFunc: 0},
			},
		}, importingModuleName)
		require.EqualError(t, err, "compilation failed: some compilation error")

		// hm.dependentCount must be intact as the instantiation failed.
		require.Zero(t, hm.dependentCount)
	})

	t.Run("start func failed", func(t *testing.T) {
		s := newStore()
		engine := s.engine.(*mockEngine)
		engine.callFailIndex = 1

		_, err = s.Instantiate(m, importedModuleName)
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
		require.EqualError(t, err, "start function[1] failed: call failed")

		// hm.dependentCount must stay incremented as the instantiation itself has already succeeded.
		require.Equal(t, 1, hm.dependentCount)
	})
}

func TestStore_ExportImportedHostFunction(t *testing.T) {
	m, err := NewHostModule("", map[string]interface{}{"host_fn": func(wasm.Module) {}})
	require.NoError(t, err)

	s := newStore()

	// Add the host module
	_, err = s.Instantiate(m, "")
	require.NoError(t, err)

	t.Run("Module is the importing module", func(t *testing.T) {
		_, err = s.Instantiate(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Type: ExternTypeFunc, Name: "host_fn", DescFunc: 0}},
			MemorySection: &Memory{Min: 1},
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

	// Add the host module
	functionName := "fn"
	m, err := NewHostModule("host",
		map[string]interface{}{functionName: func(wasm.Module) {}},
	)
	require.NoError(t, err)

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
			store := NewStore(storeCtx, &mockEngine{shouldCompileFail: false, callFailIndex: -1}, Features20191205)

			// Add the host module
			hm, err := store.Instantiate(m, "host")
			require.NoError(t, err)

			// Make a module to import the function
			mod, err := store.Instantiate(&Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{{
					Type:     ExternTypeFunc,
					Module:   hm.module.Name,
					Name:     functionName,
					DescFunc: 0,
				}},
				MemorySection: &Memory{Min: 1},
				ExportSection: map[string]*Export{functionName: {Type: ExternTypeFunc, Name: functionName, Index: 0}},
			}, "test")
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			_, err = mod.ExportedFunction(functionName).Call(mod.WithContext(tc.ctx))
			require.NoError(t, err)

			modEngine := store.modules["test"].Engine.(*mockModuleEngine)
			if tc.expected == nil {
				require.Nil(t, modEngine.ctx)
			} else {
				require.Equal(t, tc.expected, modEngine.ctx.Context())
			}
		})
	}
}

type mockEngine struct {
	shouldCompileFail bool
	callFailIndex     int
}

type mockModuleEngine struct {
	ctx           *ModuleContext
	callFailIndex int
}

func newStore() *Store {
	return NewStore(context.Background(), &mockEngine{shouldCompileFail: false, callFailIndex: -1}, Features20191205)
}

func (e *mockEngine) Compile(_, _ []*FunctionInstance) (ModuleEngine, error) {
	if e.shouldCompileFail {
		return nil, fmt.Errorf("some compilation error")
	}
	return &mockModuleEngine{callFailIndex: e.callFailIndex}, nil
}

func (e *mockModuleEngine) FunctionAddress(index Index) uintptr {
	return uintptr(index)
}

func (e *mockModuleEngine) Call(ctx *ModuleContext, f *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	if e.callFailIndex >= 0 && f.Index == Index(e.callFailIndex) {
		return nil, fmt.Errorf("call failed")
	}
	e.ctx = ctx
	return
}

func (e *mockModuleEngine) Close() {}

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

func TestStore_resolveImports(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("module not instantiated", func(t *testing.T) {
		s := newStore()
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: "unknown", Name: "unknown"}}})
		require.EqualError(t, err, "module[unknown] not instantiated")
	})
	t.Run("export instance not found", func(t *testing.T) {
		s := newStore()
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{}, Name: moduleName}
		_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: "unknown"}}})
		require.EqualError(t, err, "\"unknown\" is not exported in module \"test\"")
	})
	t.Run("imported module closed while resolving imports", func(t *testing.T) {
		s := newStore()
		m := &ModuleInstance{Exports: map[string]*ExportInstance{"export": {Function: &FunctionInstance{Type: &FunctionType{Results: []ValueType{}}}}}, Name: moduleName}
		s.modules[moduleName] = m
		m.closed = true
		_, _, _, _, _, err := s.resolveImports(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Module: moduleName, DescFunc: 0, Name: "export"}}},
		)
		require.EqualError(t, err, "trying to import module[test] but is already closed")
	})
	t.Run("func", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			f := &FunctionInstance{Type: &FunctionType{Results: []ValueType{ValueTypeF32}}}
			g := &FunctionInstance{Type: &FunctionType{Results: []ValueType{ValueTypeI32}}}
			s.modules[moduleName] = &ModuleInstance{
				Exports: map[string]*ExportInstance{
					name: {Function: f},
					"":   {Function: g},
				},
				Name: moduleName,
			}
			m := &Module{
				TypeSection: []*FunctionType{{Results: []ValueType{ValueTypeF32}}, {Results: []ValueType{ValueTypeI32}}},
				ImportSection: []*Import{
					{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 0},
					{Module: moduleName, Name: "", Type: ExternTypeFunc, DescFunc: 1},
				},
			}
			functions, _, _, _, moduleImports, err := s.resolveImports(m)
			require.NoError(t, err)
			require.Contains(t, moduleImports, s.modules[moduleName])
			require.Contains(t, functions, f)
			require.Contains(t, functions, g)
			// DependentCount must be one even when importing two instances.
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
		t.Run("type out of range", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeFunc, DescFunc: 100}}})
			require.EqualError(t, err, "import[0] func[test.target]: function type out of range")
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
			require.EqualError(t, err, "import[0] func[test.target]: signature mismatch: v_f32 != v_v")
		})
	})
	t.Run("global", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			inst := &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {Type: ExternTypeGlobal, Global: inst}}, Name: moduleName}
			_, globals, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: inst.Type}}})
			require.NoError(t, err)
			require.Contains(t, globals, inst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
		t.Run("mutability mismatch", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{Mutable: false}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{Mutable: true}}}})
			require.EqualError(t, err, "import[0] global[test.target]: mutability mismatch: true != false")
		})
		t.Run("type mismatch", func(t *testing.T) {
			s := newStore()
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeGlobal,
				Global: &GlobalInstance{Type: &GlobalType{ValType: ValueTypeI32}},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeF64}}}})
			require.EqualError(t, err, "import[0] global[test.target]: value type mismatch: f64 != i32")
		})
	})
	t.Run("table", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			tableInst := &TableInstance{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: tableInst,
			}}, Name: moduleName}
			_, _, table, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &Table{Max: &max}}}})
			require.NoError(t, err)
			require.Equal(t, table, tableInst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importTableType := &Table{Min: 2}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.EqualError(t, err, "import[0] table[test.target]: minimum size mismatch: 2 > 1")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importTableType := &Table{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
			require.EqualError(t, err, "import[0] table[test.target]: maximum size mismatch: 10, but actual has no max")
		})
	})
	t.Run("memory", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			memoryInst := &MemoryInstance{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: memoryInst,
			}}, Name: moduleName}
			_, _, _, memory, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: &Memory{Max: &max}}}})
			require.NoError(t, err)
			require.Equal(t, memory, memoryInst)
			require.Equal(t, 1, s.modules[moduleName].dependentCount)
		})
		t.Run("minimum size mismatch", func(t *testing.T) {
			s := newStore()
			importMemoryType := &Memory{Min: 2}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{Min: importMemoryType.Min - 1},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.EqualError(t, err, "import[0] memory[test.target]: minimum size mismatch: 2 > 1")
		})
		t.Run("maximum size mismatch", func(t *testing.T) {
			s := newStore()
			max := uint32(10)
			importMemoryType := &Memory{Max: &max}
			s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
				Type:   ExternTypeMemory,
				Memory: &MemoryInstance{},
			}}, Name: moduleName}
			_, _, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeMemory, DescMem: importMemoryType}}})
			require.EqualError(t, err, "import[0] memory[test.target]: maximum size mismatch: 10, but actual has no max")
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
		Table:     &TableInstance{Table: make([]uintptr, 10)},
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
		Table:     &TableInstance{Table: make([]uintptr, 10)},
		Functions: make([]*FunctionInstance, 10),
		Engine:    &mockModuleEngine{},
	}
	targetIndex, targetOffset := uint32(1), byte(0)
	targetIndex2, targetOffset2 := functionCounts-1, byte(0x8)
	m.Functions[targetIndex] = &FunctionInstance{Type: &FunctionType{}, Index: Index(targetIndex)}
	m.Functions[targetIndex2] = &FunctionInstance{Type: &FunctionType{}, Index: Index(targetIndex2)}
	m.applyElements([]*ElementSegment{
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset}}, Init: []uint32{targetIndex}},
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset2}}, Init: []uint32{targetIndex2}},
	})
	require.Equal(t, uintptr(targetIndex), m.Table.Table[targetOffset])
	require.Equal(t, uintptr(targetIndex2), m.Table.Table[targetOffset2])
}

func TestModuleInstance_decDependentCount(t *testing.T) {
	count := 100
	m := ModuleInstance{dependentCount: count}

	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			m.decDependentCount()
		}()
	}
	wg.Wait()
	require.Zero(t, m.dependentCount)
}

func TestModuleInstance_incDependentCount(t *testing.T) {
	count := 100
	m := ModuleInstance{}
	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			m.incDependentCount()
		}()
	}
	wg.Wait()
	require.Equal(t, count, m.dependentCount)
}
