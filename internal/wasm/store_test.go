package internalwasm

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/wasm"
)

func TestStore_GetModuleInstance(t *testing.T) {
	name := "test"

	s := NewStore(nopEngineInstance)

	m1 := s.getModuleInstance(name)
	require.Equal(t, m1, s.ModuleInstances[name])
	require.NotNil(t, m1.Exports)

	m2 := s.getModuleInstance(name)
	require.Equal(t, m1, m2)
}

func TestStore_CallFunction(t *testing.T) {
	name := "test"
	fn := "fn"
	engine := &nopEngine{}
	s := NewStore(engine)
	m := &ModuleInstance{
		Name: name,
		Exports: map[string]*ExportInstance{
			fn: {
				Kind: ExportKindFunc,
				Function: &FunctionInstance{
					FunctionType: &TypeInstance{
						Type: &FunctionType{
							Params:  []ValueType{},
							Results: []ValueType{},
						},
					},
				},
			},
		},
	}
	ctx := NewHostFunctionCallContext(s, m)
	s.ModuleInstances[name] = m
	s.HostFunctionCallContexts[name] = ctx

	type testKey struct{}
	ctxVal := context.WithValue(context.Background(), testKey{}, "test")

	tests := []struct {
		name      string
		ctx       context.Context
		actualCtx context.Context
	}{
		{
			name:      "nil context",
			ctx:       nil,
			actualCtx: context.Background(),
		},
		{
			name:      "background context",
			ctx:       context.Background(),
			actualCtx: context.Background(),
		},
		{
			name:      "context with value",
			ctx:       ctxVal,
			actualCtx: ctxVal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.CallFunction(tt.ctx, name, fn)
			require.NoError(t, err)
			require.Equal(t, tt.actualCtx, engine.ctx.ctx)
		})
	}
}

func TestStore_AddHostFunction(t *testing.T) {
	s := NewStore(nopEngineInstance)
	hostFunction := func(wasm.HostFunctionCallContext) {
	}

	hf := newHostFunction(t, "fn", hostFunction)
	err := s.AddHostFunction("test", hf)
	require.NoError(t, err)

	// The function was added to the store, prefixed by the owning module name
	require.Equal(t, 1, len(s.Functions))
	fn := s.Functions[0]
	require.Equal(t, "test.fn", fn.Name)

	// The function was exported in the module
	m := s.getModuleInstance("test")
	require.Equal(t, 1, len(m.Exports))
	exp, ok := m.Exports["fn"]
	require.True(t, ok)

	// Trying to register it again should fail
	err = s.AddHostFunction("test", hf)
	require.EqualError(t, err, `"fn" is already exported in module "test"`)

	// Any side effects should be reverted
	require.Equal(t, []*FunctionInstance{fn}, s.Functions)
	require.Equal(t, map[string]*ExportInstance{"fn": exp}, m.Exports)
}

func newHostFunction(t *testing.T, name string, hostFunction interface{}) *HostFunction {
	hf := &HostFunction{Name: name}
	goFn := reflect.ValueOf(hostFunction)
	hf.GoFunc = &goFn
	ft, err := GetFunctionType(hf.Name, hf.GoFunc)
	require.NoError(t, err)
	hf.FunctionType = ft
	return hf
}

func TestStore_ExportImportedHostFunction(t *testing.T) {
	s := NewStore(nopEngineInstance)
	hostFunction := func(wasm.HostFunctionCallContext) {
	}
	err := s.AddHostFunction("", newHostFunction(t, "host_fn", hostFunction))
	require.NoError(t, err)

	t.Run("ModuleInstance is the importing module", func(t *testing.T) {
		_, err = s.Instantiate(&Module{
			TypeSection:   []*FunctionType{{}},
			ImportSection: []*Import{{Kind: ImportKindFunc, Name: "host_fn", DescFunc: 0}},
			MemorySection: []*MemoryType{{1, nil}},
			ExportSection: map[string]*Export{"host.fn": {Kind: ExportKindFunc, Name: "host.fn", Index: 0}},
		}, "test")
		require.NoError(t, err)

		ei, err := s.getExport("test", "host.fn", ExportKindFunc)
		require.NoError(t, err)
		os.Environ()
		// We expect the host function to be called in context of the importing module.
		// Otherwise, it would be the pseudo-module of the host, which only includes types and function definitions.
		// Notably, this ensures the host function call context has the correct memory (from the importing module).
		require.Equal(t, s.ModuleInstances["test"], ei.Function.ModuleInstance)
	})
}

func TestStore_BuildFunctionInstances_FunctionNames(t *testing.T) {
	name := "test"

	s := NewStore(nopEngineInstance)
	mi := s.getModuleInstance(name)

	zero := Index(0)
	nopCode := &Code{nil, []byte{OpcodeEnd}}
	m := &Module{
		TypeSection:     []*FunctionType{{}},
		FunctionSection: []Index{zero, zero, zero, zero, zero},
		NameSection: &NameSection{
			FunctionNames: NameMap{
				{Index: Index(1), Name: "two"},
				{Index: Index(3), Name: "four"},
				{Index: Index(4), Name: "five"},
			},
		},
		CodeSection: []*Code{nopCode, nopCode, nopCode, nopCode, nopCode},
	}

	_, err := s.buildFunctionInstances(m, mi)
	require.NoError(t, err)

	var names []string
	for _, f := range mi.Functions {
		names = append(names, f.Name)
	}

	// We expect unknown for any functions missing data in the NameSection
	require.Equal(t, []string{"unknown", "two", "unknown", "four", "five"}, names)
}

var nopEngineInstance Engine = &nopEngine{}

type nopEngine struct {
	ctx *HostFunctionCallContext
}

func (e *nopEngine) Call(ctx *HostFunctionCallContext, _ *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	e.ctx = ctx
	return nil, nil
}

func (e *nopEngine) Compile(_ *FunctionInstance) error {
	return nil
}

func TestStore_addHostFunction(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		const max = 10
		s.maximumFunctionAddress = max
		s.Functions = make([]*FunctionInstance, max)
		err := s.addFunctionInstance(nil)
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		for i := 0; i < 10; i++ {
			f := &FunctionInstance{}
			require.Len(t, s.Functions, i)

			err := s.addFunctionInstance(f)
			require.NoError(t, err)

			// After the addition, one instance is added.
			require.Len(t, s.Functions, i+1)

			// The added function instance must have i for its address.
			require.Equal(t, FunctionAddress(i), f.Address)
		}
	})
}

func TestStore_getTypeInstance(t *testing.T) {
	t.Run("too many functions", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		const max = 10
		s.maximumFunctionTypes = max
		s.TypeIDs = make(map[string]FunctionTypeID)
		for i := 0; i < max; i++ {
			s.TypeIDs[strconv.Itoa(i)] = 0
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
				s := NewStore(nopEngineInstance)
				actual, err := s.getTypeInstance(tc)
				require.NoError(t, err)

				expectedTypeID, ok := s.TypeIDs[tc.String()]
				require.True(t, ok)
				require.Equal(t, expectedTypeID, actual.TypeID)
				require.Equal(t, tc, actual.Type)
			})
		}
	})
}

func TestStore_buildGlobalInstances(t *testing.T) {
	t.Run("too many globals", func(t *testing.T) {
		// Setup a store to have the reasonably low max on globals for testing.
		s := NewStore(nopEngineInstance)
		const max = 10
		s.maximumGlobals = max

		// Module with max+1 globals must fail.
		_, err := s.buildGlobalInstances(&Module{GlobalSection: make([]*Global, max+1)}, &ModuleInstance{})
		require.Error(t, err)
	})
	t.Run("invalid constant expression", func(t *testing.T) {
		s := NewStore(nopEngineInstance)

		// Empty constant expression is invalid.
		m := &Module{GlobalSection: []*Global{{Init: &ConstantExpression{}}}}
		_, err := s.buildGlobalInstances(m, &ModuleInstance{})
		require.Error(t, err)
	})

	t.Run("global type mismatch", func(t *testing.T) {
		s := NewStore(nopEngineInstance)
		m := &Module{GlobalSection: []*Global{{
			// Global with i32.const initial value, but with type specified as f64 must be error.
			Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}},
			Type: &GlobalType{ValType: ValueTypeF64},
		}}}
		_, err := s.buildGlobalInstances(m, &ModuleInstance{})
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		global := &Global{
			Init: &ConstantExpression{Opcode: OpcodeI64Const, Data: []byte{0x11}},
			Type: &GlobalType{ValType: ValueTypeI64},
		}
		expectedValue, _, err := leb128.DecodeUint64(bytes.NewReader(global.Init.Data))
		require.NoError(t, err)

		m := &Module{GlobalSection: []*Global{global}}

		s := NewStore(nopEngineInstance)
		target := &ModuleInstance{}
		_, err = s.buildGlobalInstances(m, target)
		require.NoError(t, err)

		// A global must be added to both store and module instance.
		require.Len(t, s.Globals, 1)
		require.Len(t, target.Globals, 1)
		// Plus the added one must be same.
		require.Equal(t, s.Globals[0], target.Globals[0])

		require.Equal(t, expectedValue, s.Globals[0].Val)
	})
}

func TestStore_executeConstExpression(t *testing.T) {
	t.Run("invalid optcode", func(t *testing.T) {
		expr := &ConstantExpression{Opcode: OpcodeNop}
		_, _, err := executeConstExpression(nil, expr)
		require.Error(t, err)
	})
	t.Run("non global expr", func(t *testing.T) {
		for _, vt := range []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64} {
			t.Run(ValueTypeName(vt), func(t *testing.T) {
				t.Run("valid", func(t *testing.T) {
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

					raw, actualType, err := executeConstExpression(nil, expr)
					require.NoError(t, err)
					require.Equal(t, vt, actualType)
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
				t.Run("invalid", func(t *testing.T) {
					// Empty data must be failure.
					expr := &ConstantExpression{Data: make([]byte, 0)}
					switch vt {
					case ValueTypeI32:
						expr.Opcode = OpcodeI32Const
					case ValueTypeI64:
						expr.Opcode = OpcodeI64Const
					case ValueTypeF32:
						expr.Opcode = OpcodeF32Const
					case ValueTypeF64:
						expr.Opcode = OpcodeF64Const
					}
					_, _, err := executeConstExpression(nil, expr)
					require.Error(t, err)
				})
			})
		}
	})
	t.Run("global expr", func(t *testing.T) {
		t.Run("failed to read global index", func(t *testing.T) {
			// Empty data for global index is invalid.
			expr := &ConstantExpression{Data: make([]byte, 0), Opcode: OpcodeGlobalGet}
			_, _, err := executeConstExpression(nil, expr)
			require.Error(t, err)
		})
		t.Run("global index out of range", func(t *testing.T) {
			// Data holds the index in leb128 and this time the value exceeds len(globals) (=0).
			expr := &ConstantExpression{Data: []byte{1}, Opcode: OpcodeGlobalGet}
			globals := []*GlobalInstance{}
			_, _, err := executeConstExpression(globals, expr)
			require.Error(t, err)
		})

		t.Run("ok", func(t *testing.T) {
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

					val, actualType, err := executeConstExpression(globals, expr)
					require.NoError(t, err)
					require.Equal(t, tc.valueType, actualType)
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
						require.Equal(t, math.Float32frombits(uint32(tc.val)), actual)
					case ValueTypeF64:
						actual, ok := val.(float64)
						require.True(t, ok)
						require.Equal(t, math.Float64frombits(tc.val), actual)
					}
				})
			}
		})
	})
}
