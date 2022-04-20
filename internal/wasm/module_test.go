package wasm

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/heeus/hwazero/api"
	"github.com/heeus/hwazero/internal/leb128"
	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/u64"
)

func TestFunctionType_String(t *testing.T) {
	for _, tc := range []struct {
		functype *FunctionType
		exp      string
	}{
		{functype: &FunctionType{}, exp: "v_v"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32}}, exp: "i32_v"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeF64}}, exp: "i32f64_v"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "f32i32f64_v"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeI64}}, exp: "v_i64"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeI64, ValueTypeF32}}, exp: "v_i64f32"},
		{functype: &FunctionType{Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "v_f32i32f64"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}}, exp: "i32_i64"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32}, Results: []ValueType{ValueTypeI64, ValueTypeF32}}, exp: "i64f32_i64f32"},
		{functype: &FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32, ValueTypeF64}, Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}, exp: "i64f32f64_f32i32f64"},
	} {
		tc := tc
		t.Run(tc.functype.String(), func(t *testing.T) {
			require.Equal(t, tc.exp, tc.functype.String())
			require.Equal(t, tc.exp, tc.functype.key())
			require.Equal(t, tc.exp, tc.functype.string)
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
		{"host_function", SectionIDHostFunction, "host_function"},
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
		expectedMemory    *Memory
		expectedTable     *Table
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
				ImportSection: []*Import{{Type: ExternTypeMemory, DescMem: &Memory{Min: 1, Max: 10}}},
			},
			expectedMemory: &Memory{Min: 1, Max: 10},
		},
		{
			module: &Module{
				MemorySection: &Memory{Min: 100},
			},
			expectedMemory: &Memory{Min: 100},
		},
		// Tables.
		{
			module: &Module{
				ImportSection: []*Import{{Type: ExternTypeTable, DescTable: &Table{Min: 1}}},
			},
			expectedTable: &Table{Min: 1},
		},
		{
			module: &Module{
				TableSection: &Table{Min: 10},
			},
			expectedTable: &Table{Min: 10},
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			functions, globals, memory, table, err := tc.module.AllDeclarations()
			require.NoError(t, err)
			require.Equal(t, tc.expectedFunctions, functions)
			require.Equal(t, tc.expectedGlobals, globals)
			require.Equal(t, tc.expectedTable, table)
			require.Equal(t, tc.expectedMemory, memory)
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

func TestModule_Validate_Errors(t *testing.T) {
	zero := Index(0)
	fn := reflect.ValueOf(func(api.Module) {})

	tests := []struct {
		name        string
		input       *Module
		expectedErr string
	}{
		{
			name: "StartSection points to an invalid func",
			input: &Module{
				TypeSection:     nil,
				FunctionSection: []uint32{0},
				CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
				StartSection:    &zero,
			},
			expectedErr: "invalid start function: func[0] has an invalid type",
		},
		{
			name: "CodeSection and HostFunctionSection",
			input: &Module{
				TypeSection:         []*FunctionType{{}},
				FunctionSection:     []uint32{0},
				CodeSection:         []*Code{{Body: []byte{OpcodeEnd}}},
				HostFunctionSection: []*reflect.Value{&fn},
			},
			expectedErr: "cannot mix functions and host functions in the same module",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.Validate(Features20191205)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
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
	t.Run("imported valid func", func(t *testing.T) {
		index := Index(1)
		m := Module{
			StartSection: &index,
			TypeSection:  []*FunctionType{{}, {Results: []ValueType{ValueTypeI32}}},
			ImportSection: []*Import{
				{Type: ExternTypeFunc, DescFunc: 1},
				// import with index 1 is global but this should be skipped when searching imported functions.
				{Type: ExternTypeGlobal},
				{Type: ExternTypeFunc, DescFunc: 0}, // This one must be selected.
			},
		}
		err := m.validateStartSection()
		require.NoError(t, err)
	})
}

func TestModule_validateGlobals(t *testing.T) {
	t.Run("too many globals", func(t *testing.T) {
		m := Module{}
		err := m.validateGlobals(make([]*GlobalType, 10), 9)
		require.Error(t, err)
		require.EqualError(t, err, "too many globals in a module")
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
		require.EqualError(t, err, "global index out of range")
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
		require.EqualError(t, err, "invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{GlobalSection: []*Global{
			{
				Type: &GlobalType{ValType: ValueTypeI32},
				Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
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
	t.Run("ok", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []uint32{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeI32Const, 0, OpcodeDrop, OpcodeEnd}}},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.NoError(t, err)
	})
	t.Run("too many functions", func(t *testing.T) {
		m := Module{}
		err := m.validateFunctions(Features20191205, []uint32{1, 2, 3, 4}, nil, nil, nil, 3)
		require.Error(t, err)
		require.EqualError(t, err, "too many functions in a store")
	})
	t.Run("function, but no code", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []Index{0},
			CodeSection:     nil,
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.EqualError(t, err, "code count (0) != function count (1)")
	})
	t.Run("function out of range of code", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []Index{1},
			CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.EqualError(t, err, "invalid function[0]: type section index 1 out of range")
	})
	t.Run("invalid", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []Index{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeF32Abs}}},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid function[0]: cannot pop the 1st f32 operand")
	})
	t.Run("in- exported", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []Index{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection:   []*Export{{Name: "f1", Type: ExternTypeFunc, Index: 0}},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1"]: cannot pop the 1st f32`)
	})
	t.Run("in- exported after import", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			ImportSection:   []*Import{{Type: ExternTypeFunc}},
			FunctionSection: []Index{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection:   []*Export{{Name: "f1", Type: ExternTypeFunc, Index: 1}},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1"]: cannot pop the 1st f32`)
	})
	t.Run("in- exported twice", func(t *testing.T) {
		m := Module{
			TypeSection:     []*FunctionType{{}},
			FunctionSection: []Index{0},
			CodeSection:     []*Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection: []*Export{
				{Name: "f1", Type: ExternTypeFunc, Index: 0},
				{Name: "f2", Type: ExternTypeFunc, Index: 0},
			},
		}
		err := m.validateFunctions(Features20191205, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1","f2"]: cannot pop the 1st f32`)
	})
}

func TestModule_validateMemory(t *testing.T) {
	t.Run("data section exits but memory not declared", func(t *testing.T) {
		m := Module{DataSection: make([]*DataSegment, 1)}
		err := m.validateMemory(nil, nil)
		require.Error(t, err)
		require.Contains(t, "unknown memory", err.Error())
	})
	t.Run("invalid const expr", func(t *testing.T) {
		m := Module{DataSection: []*DataSegment{{
			OffsetExpression: &ConstantExpression{
				Opcode: OpcodeUnreachable, // Invalid!
			},
		}}}
		err := m.validateMemory(&Memory{}, nil)
		require.EqualError(t, err, "calculate offset: invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{DataSection: []*DataSegment{{
			Init: []byte{0x1},
			OffsetExpression: &ConstantExpression{
				Opcode: OpcodeI32Const,
				Data:   leb128.EncodeInt32(1),
			},
		}}}
		err := m.validateMemory(&Memory{}, nil)
		require.NoError(t, err)
	})
}

func TestModule_validateImports(t *testing.T) {
	for _, tc := range []struct {
		name            string
		enabledFeatures Features
		i               *Import
		expectedErr     string
	}{
		{name: "empty import section"},
		{
			name:            "func",
			enabledFeatures: Features20191205,
			i:               &Import{Module: "m", Name: "n", Type: ExternTypeFunc, DescFunc: 0},
		},
		{
			name:            "global var disabled",
			enabledFeatures: Features20191205.Set(FeatureMutableGlobal, false),
			i: &Import{
				Module:     "m",
				Name:       "n",
				Type:       ExternTypeGlobal,
				DescGlobal: &GlobalType{ValType: ValueTypeI32, Mutable: true},
			},
			expectedErr: `invalid import["m"."n"] global: feature "mutable-global" is disabled`,
		},
		{
			name:            "table",
			enabledFeatures: Features20191205,
			i: &Import{
				Module:    "m",
				Name:      "n",
				Type:      ExternTypeTable,
				DescTable: &Table{Min: 1},
			},
		},
		{
			name:            "memory",
			enabledFeatures: Features20191205,
			i: &Import{
				Module:  "m",
				Name:    "n",
				Type:    ExternTypeMemory,
				DescMem: &Memory{Min: 1},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := Module{}
			if tc.i != nil {
				m.ImportSection = []*Import{tc.i}
			}
			err := m.validateImports(tc.enabledFeatures)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_validateExports(t *testing.T) {
	for _, tc := range []struct {
		name            string
		enabledFeatures Features
		exportSection   []*Export
		functions       []Index
		globals         []*GlobalType
		memory          *Memory
		table           *Table
		expectedErr     string
	}{
		{name: "empty export section", exportSection: []*Export{}},
		{
			name:            "func",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeFunc, Index: 0}},
			functions:       []Index{100 /* arbitrary type id*/},
		},
		{
			name:            "func out of range",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeFunc, Index: 1, Name: "e"}},
			functions:       []Index{100 /* arbitrary type id*/},
			expectedErr:     `unknown function for export["e"]`,
		},
		{
			name:            "global const",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeGlobal, Index: 0}},
			globals:         []*GlobalType{{ValType: ValueTypeI32}},
		},
		{
			name:            "global var",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeGlobal, Index: 0}},
			globals:         []*GlobalType{{ValType: ValueTypeI32, Mutable: true}},
		},
		{
			name:            "global var disabled",
			enabledFeatures: Features20191205.Set(FeatureMutableGlobal, false),
			exportSection:   []*Export{{Type: ExternTypeGlobal, Index: 0, Name: "e"}},
			globals:         []*GlobalType{{ValType: ValueTypeI32, Mutable: true}},
			expectedErr:     `invalid export["e"] global[0]: feature "mutable-global" is disabled`,
		},
		{
			name:            "global out of range",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeGlobal, Index: 1, Name: "e"}},
			globals:         []*GlobalType{{}},
			expectedErr:     `unknown global for export["e"]`,
		},
		{
			name:            "table",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeTable, Index: 0}},
			table:           &Table{},
		},
		{
			name:            "table out of range",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeTable, Index: 1, Name: "e"}},
			table:           &Table{},
			expectedErr:     `table for export["e"] out of range`,
		},
		{
			name:            "memory",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeMemory, Index: 0}},
			memory:          &Memory{},
		},
		{
			name:            "memory out of range",
			enabledFeatures: Features20191205,
			exportSection:   []*Export{{Type: ExternTypeMemory, Index: 0, Name: "e"}},
			table:           &limitsType{},
			expectedErr:     `memory for export["e"] out of range`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := Module{ExportSection: tc.exportSection}
			err := m.validateExports(tc.enabledFeatures, tc.functions, tc.globals, tc.memory, tc.table)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_buildGlobalInstances(t *testing.T) {
	m := Module{GlobalSection: []*Global{
		{
			Type: &GlobalType{Mutable: true, ValType: ValueTypeF64},
			Init: &ConstantExpression{Opcode: OpcodeF64Const,
				Data: u64.LeBytes(api.EncodeF64(math.MaxFloat64))},
		},
		{
			Type: &GlobalType{Mutable: false, ValType: ValueTypeI32},
			Init: &ConstantExpression{Opcode: OpcodeI32Const,
				Data: leb128.EncodeInt32(math.MaxInt32)},
		},
	}}

	globals := m.buildGlobals(nil)
	expectedGlobals := []*GlobalInstance{
		{Type: &GlobalType{ValType: ValueTypeF64, Mutable: true}, Val: api.EncodeF64(math.MaxFloat64)},
		{Type: &GlobalType{ValType: ValueTypeI32, Mutable: false}, Val: math.MaxInt32},
	}
	require.Equal(t, expectedGlobals, globals)
}

func TestModule_buildFunctionInstances(t *testing.T) {
	nopCode := &Code{nil, []byte{OpcodeEnd}}
	m := Module{
		TypeSection:   []*FunctionType{{}},
		ImportSection: []*Import{{Type: ExternTypeFunc}},
		NameSection: &NameSection{
			FunctionNames: NameMap{
				{Index: Index(2), Name: "two"},
				{Index: Index(4), Name: "four"},
				{Index: Index(5), Name: "five"},
			},
		},
		FunctionSection: []Index{0, 0, 0, 0, 0},
		CodeSection:     []*Code{nopCode, nopCode, nopCode, nopCode, nopCode},
	}

	// Note: This only returns module-defined functions, not imported ones. That's why the index starts with 1, not 0.
	actual := m.buildFunctions("counter")
	expectedNames := []string{"counter.[1]", "counter.two", "counter.[3]", "counter.four", "counter.five"}
	for i, f := range actual {
		require.Equal(t, expectedNames[i], f.DebugName)
		require.Equal(t, nopCode.Body, f.Body)
	}
}

func TestModule_buildMemoryInstance(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		m := Module{}
		mem := m.buildMemory()
		require.Nil(t, mem)
	})
	t.Run("non-nil", func(t *testing.T) {
		min := uint32(1)
		max := uint32(10)
		m := Module{MemorySection: &Memory{Min: min, Max: max}}
		mem := m.buildMemory()
		require.Equal(t, min, mem.Min)
		require.Equal(t, max, mem.Max)
	})
}
