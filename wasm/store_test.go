package wasm

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm/internal/leb128"
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

func TestStore_AddHostFunction(t *testing.T) {
	s := NewStore(nopEngineInstance)
	hostFunction := func(_ *HostFunctionCallContext) {
	}

	err := s.AddHostFunction("test", "fn", reflect.ValueOf(hostFunction))
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

	// Trying to add it again should fail
	hostFunction2 := func(_ *HostFunctionCallContext) {
	}
	err = s.AddHostFunction("test", "fn", reflect.ValueOf(hostFunction2))
	require.EqualError(t, err, `"fn" is already exported in module "test"`)

	// Any side effects should be reverted
	require.Equal(t, []*FunctionInstance{fn}, s.Functions)
	require.Equal(t, map[string]*ExportInstance{"fn": exp}, m.Exports)
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
}

func (e *nopEngine) Call(_ *FunctionInstance, _ ...uint64) (results []uint64, err error) {
	return nil, nil
}

func (e *nopEngine) Compile(_ *FunctionInstance) error {
	return nil
}

func TestStore_addFunctionInstance(t *testing.T) {
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

			// The added function instance must have i for its funcaddr.
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

func TestMemoryInstance_ValidateAddrRange(t *testing.T) {
	memory := &MemoryInstance{
		Buffer: make([]byte, 100),
	}

	tests := []struct {
		name      string
		addr      uint32
		rangeSize uint64
		expected  bool
	}{
		{
			name:      "simple valid arguments",
			addr:      0,   // arbitrary valid address
			rangeSize: 100, // arbitrary valid size
			expected:  true,
		},
		{
			name:      "maximum valid rangeSize",
			addr:      0, // arbitrary valid address
			rangeSize: uint64(len(memory.Buffer)),
			expected:  true,
		},
		{
			name:      "rangeSize exceeds the valid size by 1",
			addr:      100, // arbitrary valid address
			rangeSize: uint64(len(memory.Buffer)) - 99,
			expected:  false,
		},
		{
			name:      "rangeSize exceeds the valid size and the memory size by 1",
			addr:      0, // arbitrary valid address
			rangeSize: uint64(len(memory.Buffer)) + 1,
			expected:  false,
		},
		{
			name:      "addr exceeds the memory size",
			addr:      uint32(len(memory.Buffer)),
			rangeSize: 0, // arbitrary size
			expected:  false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, memory.ValidateAddrRange(tc.addr, tc.rangeSize))
		})
	}
}

func TestMemoryInstance_PutUint32(t *testing.T) {
	memory := &MemoryInstance{
		Buffer: make([]byte, 100),
	}

	tests := []struct {
		name               string
		addr               uint32
		val                uint32
		shouldSuceed       bool
		expectedWrittenVal uint32
	}{
		{
			name:               "valid addr with an endian-insensitive val",
			addr:               0, // arbitrary valid address.
			val:                0xffffffff,
			shouldSuceed:       true,
			expectedWrittenVal: 0xffffffff,
		},
		{
			name:               "valid addr with an endian-sensitive val",
			addr:               0, // arbitrary valid address.
			val:                0xfffffffe,
			shouldSuceed:       true,
			expectedWrittenVal: 0xfffffffe,
		},
		{
			name:               "maximum boundary valid addr",
			addr:               uint32(len(memory.Buffer)) - 4, // 4 is the size of uint32
			val:                1,                              // arbitrary valid val
			shouldSuceed:       true,
			expectedWrittenVal: 1,
		},
		{
			name:         "addr exceeds the maximum valid addr by 1",
			addr:         uint32(len(memory.Buffer)) - 4 + 1, // 4 is the size of uint32
			val:          1,                                  // arbitrary valid val
			shouldSuceed: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.shouldSuceed, memory.PutUint32(tc.addr, tc.val))
			if tc.shouldSuceed {
				require.Equal(t, tc.expectedWrittenVal, binary.LittleEndian.Uint32(memory.Buffer[tc.addr:tc.addr+4])) // 4 is the size of uint32
			}
		})
	}
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
			t.Run(valueTypeName(vt), func(t *testing.T) {
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
				t.Run(valueTypeName(tc.valueType), func(t *testing.T) {
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
