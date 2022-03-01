package internalwasm

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionType_String(t *testing.T) {
	for _, tc := range []struct {
		functype *FunctionType
		exp      string
	}{
		{functype: &FunctionType{}, exp: "null_null"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32}}, exp: "i32_null"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeF64}}, exp: "i32f64_null"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "f32i32f64_null"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeI64}}, exp: "null_i64"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeI64, ValueTypeF32}}, exp: "null_i64f32"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "null_f32i32f64"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}}, exp: "i32_i64"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32}, Results: []ValueType{ValueTypeI64, ValueTypeF32}}, exp: "i64f32_i64f32"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32, ValueTypeF64}, Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "i64f32f64_f32i32f64"},
	} {
		tc := tc
		t.Run(tc.functype.String(), func(t *testing.T) {
			require.Equal(t, tc.exp, tc.functype.String())
		})
	}
}

func TestSectionIDName(t *testing.T) {
	tests := []struct {
		name     string
		input    SectionID
		expected string
	}{
		{"custom", SectionIDCustom, "custom"},
		{"type", SectionIDType, "type"},
		{"import", SectionIDImport, "import"},
		{"function", SectionIDFunction, "function"},
		{"table", SectionIDTable, "table"},
		{"memory", SectionIDMemory, "memory"},
		{"global", SectionIDGlobal, "global"},
		{"export", SectionIDExport, "export"},
		{"start", SectionIDStart, "start"},
		{"element", SectionIDElement, "element"},
		{"code", SectionIDCode, "code"},
		{"data", SectionIDData, "data"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, SectionIDName(tc.input))
		})
	}
}

func TestExternTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    ExternType
		expected string
	}{
		{"func", ExternTypeFunc, "func"},
		{"table", ExternTypeTable, "table"},
		{"mem", ExternTypeMemory, "memory"},
		{"global", ExternTypeGlobal, "global"},
		{"unknown", 100, "0x64"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ExternTypeName(tc.input))
		})
	}
}

func TestModule_allDeclarations(t *testing.T) {
	for i, tc := range []struct {
		module            *Module
		expectedFunctions []Index
		expectedGlobals   []*GlobalType
		expectedMemories  []*MemoryType
		expectedTables    []*TableType
	}{
		// Functions.
		{
			module: &Module{
				ImportSection:   []*Import{{Type: ExternTypeFunc, DescFunc: 10000}},
				FunctionSection: []Index{10, 20, 30},
			},
			expectedFunctions: []Index{10000, 10, 20, 30},
		},
		{
			module: &Module{
				FunctionSection: []Index{10, 20, 30},
			},
			expectedFunctions: []Index{10, 20, 30},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeFunc, DescFunc: 10000}},
			},
			expectedFunctions: []Index{10000},
		},
		// Globals.
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeGlobal, DescGlobal: &GlobalType{Mutable: false}}},
				GlobalSection: []*Global{{Type: &GlobalType{Mutable: true}}},
			},
			expectedGlobals: []*GlobalType{{Mutable: false}, {Mutable: true}},
		},
		{
			module: &Module{
				GlobalSection: []*Global{{Type: &GlobalType{Mutable: true}}},
			},
			expectedGlobals: []*GlobalType{{Mutable: true}},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeGlobal, DescGlobal: &GlobalType{Mutable: false}}},
			},
			expectedGlobals: []*GlobalType{{Mutable: false}},
		},
		// Memories.
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeMemory, DescMem: &LimitsType{Min: 1}}},
				MemorySection: []*MemoryType{{Min: 100}},
			},
			expectedMemories: []*MemoryType{{Min: 1}, {Min: 100}},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeMemory, DescMem: &LimitsType{Min: 1}}},
			},
			expectedMemories: []*MemoryType{{Min: 1}},
		},
		{
			module: &Module{
				MemorySection: []*MemoryType{{Min: 100}},
			},
			expectedMemories: []*MemoryType{{Min: 100}},
		},
		// Tables.
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeTable, DescTable: &TableType{Limit: &LimitsType{Min: 1}}}},
				TableSection:  []*TableType{{Limit: &LimitsType{Min: 10}}},
			},
			expectedTables: []*TableType{{Limit: &LimitsType{Min: 1}}, {Limit: &LimitsType{Min: 10}}},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeTable, DescTable: &TableType{Limit: &LimitsType{Min: 1}}}},
			},
			expectedTables: []*TableType{{Limit: &LimitsType{Min: 1}}},
		},
		{
			module: &Module{
				TableSection: []*TableType{{Limit: &LimitsType{Min: 10}}},
			},
			expectedTables: []*TableType{{Limit: &LimitsType{Min: 10}}},
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			functions, globals, memories, tables := tc.module.allDeclarations()
			require.Equal(t, tc.expectedFunctions, functions)
			require.Equal(t, tc.expectedGlobals, globals)
			require.Equal(t, tc.expectedTables, tables)
			require.Equal(t, tc.expectedMemories, memories)
		})
	}
}

func TestModule_SectionSize(t *testing.T) {
	i32, f32 := ValueTypeI32, ValueTypeF32
	zero := uint32(0)
	empty := &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x00}}

	tests := []struct {
		name     string
		input    *Module
		expected map[string]uint32
	}{
		{
			name:     "empty",
			input:    &Module{},
			expected: map[string]uint32{},
		},
		{
			name:     "only name section",
			input:    &Module{NameSection: &NameSection{ModuleName: "simple"}},
			expected: map[string]uint32{"custom": 1},
		},
		{
			name: "type section",
			input: &Module{
				TypeSection: []*FunctionType{
					{},
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{i32, i32, i32, i32}, Results: []ValueType{i32}},
				},
			},
			expected: map[string]uint32{"type": 3},
		},
		{
			name: "type and import section",
			input: &Module{
				TypeSection: []*FunctionType{
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{f32, f32}, Results: []ValueType{f32}},
				},
				ImportSection: []*Import{
					{
						Module: "Math", Name: "Mul",
						Type:     ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Type:     ExternTypeFunc,
						DescFunc: 0,
					},
				},
			},
			expected: map[string]uint32{"import": 2, "type": 2},
		},
		{
			name: "type function and start section",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection: []*Code{
					{Body: []byte{OpcodeLocalGet, 0, OpcodeLocalGet, 1, OpcodeI32Add, OpcodeEnd}},
				},
				ExportSection: map[string]*Export{
					"AddInt": {Name: "AddInt", Type: ExternTypeFunc, Index: Index(0)},
				},
				StartSection: &zero,
			},
			expected: map[string]uint32{"code": 1, "export": 1, "function": 1, "start": 1, "type": 1},
		},
		{
			name: "memory and data",
			input: &Module{
				MemorySection: []*MemoryType{{Min: 1}},
				DataSection:   []*DataSegment{{MemoryIndex: 0, OffsetExpression: empty}},
			},
			expected: map[string]uint32{"data": 1, "memory": 1},
		},
		{
			name: "table and element",
			input: &Module{
				TableSection:   []*TableType{{ElemType: 0x70, Limit: &LimitsType{Min: 1}}},
				ElementSection: []*ElementSegment{{TableIndex: 0, OffsetExpr: empty}},
			},
			expected: map[string]uint32{"element": 1, "table": 1},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			actual := map[string]uint32{}
			for i := SectionID(0); i <= SectionIDData; i++ {
				if size := tc.input.SectionElementCount(i); size > 0 {
					actual[SectionIDName(i)] = size
				}
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestValidateConstExpression(t *testing.T) {
	t.Run("invalid opcode", func(t *testing.T) {
		expr := &ConstantExpression{Opcode: OpcodeNop}
		err := validateConstExpression(nil, expr, valueTypeUnknown)
		require.Error(t, err)
	})
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

				err := validateConstExpression(nil, expr, vt)
				require.NoError(t, err)
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
				err := validateConstExpression(nil, expr, vt)
				require.Error(t, err)
			})
		})
	}
	t.Run("global expr", func(t *testing.T) {
		t.Run("failed to read global index", func(t *testing.T) {
			// Empty data for global index is invalid.
			expr := &ConstantExpression{Data: make([]byte, 0), Opcode: OpcodeGlobalGet}
			err := validateConstExpression(nil, expr, valueTypeUnknown)
			require.Error(t, err)
		})
		t.Run("global index out of range", func(t *testing.T) {
			// Data holds the index in leb128 and this time the value exceeds len(globals) (=0).
			expr := &ConstantExpression{Data: []byte{1}, Opcode: OpcodeGlobalGet}
			var globals []*GlobalType
			err := validateConstExpression(globals, expr, valueTypeUnknown)
			require.Error(t, err)
		})

		t.Run("type mismatch", func(t *testing.T) {
			for _, vt := range []ValueType{
				ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64,
			} {
				t.Run(ValueTypeName(vt), func(t *testing.T) {
					// The index specified in Data equals zero.
					expr := &ConstantExpression{Data: []byte{0}, Opcode: OpcodeGlobalGet}
					globals := []*GlobalType{{ValType: valueTypeUnknown}}

					err := validateConstExpression(globals, expr, vt)
					require.Error(t, err)
				})
			}
		})
		t.Run("ok", func(t *testing.T) {
			for _, vt := range []ValueType{
				ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64,
			} {
				t.Run(ValueTypeName(vt), func(t *testing.T) {
					// The index specified in Data equals zero.
					expr := &ConstantExpression{Data: []byte{0}, Opcode: OpcodeGlobalGet}
					globals := []*GlobalType{{ValType: vt}}

					err := validateConstExpression(globals, expr, vt)
					require.NoError(t, err)
				})
			}
		})
	})
}

func TestModule_validateStartSection(t *testing.T) {
	t.Run("no start section", func(t *testing.T) {
		m := Module{}
		err := m.validateStartSection()
		require.NoError(t, err)
	})

	t.Run("invalid type", func(t *testing.T) {
		for _, ft := range []*FunctionType{
			{Params: []ValueType{ValueTypeI32}},
			{Results: []ValueType{ValueTypeI32}},
			{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI32}},
		} {
			t.Run(ft.String(), func(t *testing.T) {
				index := uint32(0)
				m := Module{StartSection: &index, FunctionSection: []uint32{0}, TypeSection: []*FunctionType{ft}}
				err := m.validateStartSection()
				require.Error(t, err)
			})
		}
	})
}

func TestModule_validateGlobals(t *testing.T) {
	t.Run("too many globals", func(t *testing.T) {
		m := Module{}
		err := m.validateGlobals(make([]*GlobalType, 10), 9)
		require.Error(t, err)
		require.Contains(t, err.Error(), "too many globals")
	})
	t.Run("global index out of range", func(t *testing.T) {
		m := Module{GlobalSection: []*Global{
			{
				Type: &GlobalType{ValType: ValueTypeI32},
				// Trying to reference globals[1] which is not imported.
				Init: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{1}},
			},
		}}
		err := m.validateGlobals(nil, 9)
		require.Error(t, err)
		require.Contains(t, err.Error(), "global index out of range")
	})
	t.Run("invalid const expression", func(t *testing.T) {
		m := Module{GlobalSection: []*Global{
			{
				Type: &GlobalType{ValType: valueTypeUnknown},
				Init: &ConstantExpression{Opcode: OpcodeUnreachable},
			},
		}}
		err := m.validateGlobals(nil, 9)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid opcode for const expression")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{GlobalSection: []*Global{
			{
				Type: &GlobalType{ValType: ValueTypeI32},
				Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}},
			},
		}}
		err := m.validateGlobals(nil, 9)
		require.NoError(t, err)
	})
	t.Run("ok with imported global", func(t *testing.T) {
		m := Module{
			GlobalSection: []*Global{
				{
					Type: &GlobalType{ValType: ValueTypeI32},
					// Trying to reference globals[1] which is imported.
					Init: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0}},
				},
			},
			ImportSection: []*Import{{Type: ExternTypeGlobal}},
		}
		globalDeclarations := []*GlobalType{
			{ValType: ValueTypeI32}, // Imported one.
			nil,                     // the local one trying to validate.
		}
		err := m.validateGlobals(globalDeclarations, 9)
		require.NoError(t, err)
	})
}

func TestModule_validateFunctions(t *testing.T) {
	t.Run("type index out of range", func(t *testing.T) {
		m := Module{FunctionSection: []uint32{1000 /* arbitrary large */}}
		err := m.validateFunctions(nil, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "function type index out of range")
	})
	t.Run("insufficient code section", func(t *testing.T) {
		m := Module{
			FunctionSection: []uint32{0},
			TypeSection:     []*FunctionType{{}},
			// Code section not exists.
		}
		err := m.validateFunctions(nil, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "code index out of range")
	})
	t.Run("invalid function", func(t *testing.T) {
		m := Module{
			FunctionSection: []uint32{0},
			TypeSection:     []*FunctionType{{}},
			CodeSection: []*Code{
				{Body: []byte{OpcodeF32Abs}},
			},
		}
		err := m.validateFunctions(nil, nil, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid function (0/0): cannot pop the 1st f32 operand")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{
			FunctionSection: []uint32{0},
			TypeSection:     []*FunctionType{{}},
			CodeSection: []*Code{
				{Body: []byte{OpcodeI32Const, 0, OpcodeDrop, OpcodeEnd}},
			},
		}
		err := m.validateFunctions(nil, nil, nil, nil)
		require.NoError(t, err)
	})
}

func TestModule_validateTables(t *testing.T) {
	t.Run("multiple tables", func(t *testing.T) {
		m := Module{}
		err := m.validateTables(make([]*TableType, 10), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple tables are not supported")
	})
	t.Run("table index out of range", func(t *testing.T) {
		m := Module{ElementSection: []*ElementSegment{{TableIndex: 1000}}}
		err := m.validateTables(make([]*TableType, 1), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "table index out of range")
	})
	t.Run("invalid const expr", func(t *testing.T) {
		m := Module{ElementSection: []*ElementSegment{{
			TableIndex: 0,
			OffsetExpr: &ConstantExpression{
				Opcode: OpcodeUnreachable, // Invalid!
			},
		}}}
		err := m.validateTables(make([]*TableType, 1), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{ElementSection: []*ElementSegment{{
			TableIndex: 0,
			OffsetExpr: &ConstantExpression{
				Opcode: OpcodeI32Const,
				Data:   []byte{0x1},
			},
		}}}
		err := m.validateTables(make([]*TableType, 1), nil)
		require.NoError(t, err)
	})
}

func TestModule_validateMemories(t *testing.T) {
	t.Run("multiple memory", func(t *testing.T) {
		m := Module{}
		err := m.validateMemories(make([]*LimitsType, 10), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple memories are not supported")
	})
	t.Run("data section exits but memory not declared", func(t *testing.T) {
		m := Module{DataSection: make([]*DataSegment, 1)}
		err := m.validateMemories(nil, nil)
		require.Error(t, err)
		require.Contains(t, "unknown memory", err.Error())
	})
	t.Run("non zero memory index data", func(t *testing.T) {
		m := Module{DataSection: []*DataSegment{{MemoryIndex: 1}}}
		err := m.validateMemories(make([]*LimitsType, 1), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "memory index must be zero")
	})
	t.Run("invalid const expr", func(t *testing.T) {
		m := Module{DataSection: []*DataSegment{{
			MemoryIndex: 0,
			OffsetExpression: &ConstantExpression{
				Opcode: OpcodeUnreachable, // Invalid!
			},
		}}}
		err := m.validateMemories(make([]*LimitsType, 1), nil)
		require.Contains(t, err.Error(), "invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{DataSection: []*DataSegment{{
			MemoryIndex: 0, Init: []byte{0x1},
			OffsetExpression: &ConstantExpression{
				Opcode: OpcodeI32Const,
				Data:   []byte{0x1},
			},
		}}}
		err := m.validateMemories(make([]*LimitsType, 1), nil)
		require.NoError(t, err)
	})
}

func TestModule_validateExports(t *testing.T) {
	for _, tc := range []struct {
		name          string
		exportSection map[string]*Export
		functions     []Index
		globals       []*GlobalType
		memories      []*MemoryType
		tables        []*TableType
		expErr        bool
	}{
		{name: "empty export section", exportSection: map[string]*Export{}},
		{
			name:          "valid func",
			exportSection: map[string]*Export{"": {Type: ExternTypeFunc, Index: 0}},
			functions:     []Index{100 /* arbitrary type id*/},
		},
		{
			name:          "invalid func",
			exportSection: map[string]*Export{"": {Type: ExternTypeFunc, Index: 1}},
			functions:     []Index{100 /* arbitrary type id*/},
			expErr:        true,
		},
		{
			name:          "valid global",
			exportSection: map[string]*Export{"": {Type: ExternTypeGlobal, Index: 0}},
			globals:       []*GlobalType{{}},
		},
		{
			name:          "invalid global",
			exportSection: map[string]*Export{"": {Type: ExternTypeFunc, Index: 1}},
			globals:       []*GlobalType{{}},
			expErr:        true,
		},
		{
			name:          "valid table",
			exportSection: map[string]*Export{"": {Type: ExternTypeTable, Index: 0}},
			tables:        []*TableType{{}},
		},
		{
			name:          "invalid table",
			exportSection: map[string]*Export{"": {Type: ExternTypeTable, Index: 1}},
			tables:        []*TableType{{}},
			expErr:        true,
		},
		{
			name:          "valid memory",
			exportSection: map[string]*Export{"": {Type: ExternTypeMemory, Index: 0}},
			memories:      []*MemoryType{&LimitsType{}},
		},
		{
			name:          "invalid memory index",
			exportSection: map[string]*Export{"": {Type: ExternTypeMemory, Index: 1}},
			expErr:        true,
		},
		{
			name:          "invalid memory index",
			exportSection: map[string]*Export{"": {Type: ExternTypeMemory, Index: 1}},
			// Multiple memories are not valid.
			memories: []*MemoryType{&LimitsType{}, &LimitsType{}},
			expErr:   true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := Module{ExportSection: tc.exportSection}
			err := m.validateExports(tc.functions, tc.globals, tc.memories, tc.tables)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_buildGlobalInstances(t *testing.T) {
	data := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint64(data, math.Float64bits(1.0))
	m := Module{GlobalSection: []*Global{
		{
			Type: &GlobalType{Mutable: true, ValType: ValueTypeF64},
			Init: &ConstantExpression{Opcode: OpcodeF64Const,
				Data: []byte{0, 0, 0, 0, 0, 0, 0xf0, 0x3f}}, // == float64(1.0)
		},
		{
			Type: &GlobalType{Mutable: false, ValType: ValueTypeI32},
			Init: &ConstantExpression{Opcode: OpcodeI32Const,
				Data: []byte{1}},
		},
	}}

	globals := m.buildGlobalInstances(nil)
	expectedGlobals := []*GlobalInstance{
		{GlobalType: &GlobalType{ValType: ValueTypeF64, Mutable: true}, Val: math.Float64bits(1.0)},
		{GlobalType: &GlobalType{ValType: ValueTypeI32, Mutable: false}, Val: uint64(1)},
	}

	require.Len(t, globals, len(expectedGlobals))
	for i := range globals {
		actual, expected := globals[i], expectedGlobals[i]
		require.Equal(t, expected, actual)
	}
}

func TestModule_buildFunctionInstances(t *testing.T) {
	nopCode := &Code{nil, []byte{OpcodeEnd}}
	m := Module{
		ImportSection: []*Import{{Type: ExternTypeFunc}},
		NameSection: &NameSection{
			FunctionNames: NameMap{
				{Index: Index(2), Name: "two"},
				{Index: Index(4), Name: "four"},
				{Index: Index(5), Name: "five"},
			},
		},
		CodeSection: []*Code{nopCode, nopCode, nopCode, nopCode, nopCode},
	}

	actual := m.buildFunctionInstances()
	expectedNames := []string{"unknown", "two", "unknown", "four", "five"}
	for i, f := range actual {
		require.Equal(t, expectedNames[i], f.Name)
		require.Equal(t, nopCode.Body, f.Body)
	}
}

func TestModule_buildMemoryInstance(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		m := Module{MemorySection: []*MemoryType{}}
		mem := m.buildMemoryInstance()
		require.Nil(t, mem)
	})
	t.Run("non-nil", func(t *testing.T) {
		min := uint32(1)
		max := uint32(10)
		m := Module{MemorySection: []*MemoryType{&LimitsType{Min: min, Max: &max}}}
		mem := m.buildMemoryInstance()
		require.Equal(t, min, mem.Min)
		require.Equal(t, max, *mem.Max)
	})
}

func TestModule_buildTableInstance(t *testing.T) {
	m := Module{TableSection: []*TableType{{Limit: &LimitsType{Min: 1}}}}
	table := m.buildTableInstance()
	require.Equal(t, uint32(1), table.Min)
}
