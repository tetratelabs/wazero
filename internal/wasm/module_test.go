package wasm

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
)

func TestFunctionType_String(t *testing.T) {
	tests := []struct {
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
	}

	for _, tt := range tests {
		tc := tt
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
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, SectionIDName(tc.input))
		})
	}
}

func TestMemory_Validate(t *testing.T) {
	tests := []struct {
		name        string
		mem         *Memory
		expectedErr string
	}{
		{
			name: "ok",
			mem:  &Memory{Min: 2, Cap: 2, Max: 2},
		},
		{
			name:        "cap < min",
			mem:         &Memory{Min: 2, Cap: 1, Max: 2},
			expectedErr: "capacity 1 pages (64 Ki) less than minimum 2 pages (128 Ki)",
		},
		{
			name:        "cap > maxLimit",
			mem:         &Memory{Min: 2, Cap: math.MaxUint32, Max: 2},
			expectedErr: "capacity 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "max < min",
			mem:         &Memory{Min: 2, Cap: 2, Max: 0, IsMaxEncoded: true},
			expectedErr: "min 2 pages (128 Ki) > max 0 pages (0 Ki)",
		},
		{
			name:        "min > limit",
			mem:         &Memory{Min: math.MaxUint32},
			expectedErr: "min 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "max > limit",
			mem:         &Memory{Max: math.MaxUint32, IsMaxEncoded: true},
			expectedErr: "max 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := tc.mem.Validate(MemoryLimitPages)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestModule_allDeclarations(t *testing.T) {
	tests := []struct {
		module            *Module
		expectedFunctions []Index
		expectedGlobals   []GlobalType
		expectedMemory    *Memory
		expectedTables    []Table
	}{
		// Functions.
		{
			module: &Module{
				ImportSection:   []Import{{Type: ExternTypeFunc, DescFunc: 10000}},
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
				ImportSection: []Import{{Type: ExternTypeFunc, DescFunc: 10000}},
			},
			expectedFunctions: []Index{10000},
		},
		// Globals.
		{
			module: &Module{
				ImportSection: []Import{{Type: ExternTypeGlobal, DescGlobal: GlobalType{Mutable: false}}},
				GlobalSection: []Global{{Type: GlobalType{Mutable: true}}},
			},
			expectedGlobals: []GlobalType{{Mutable: false}, {Mutable: true}},
		},
		{
			module: &Module{
				GlobalSection: []Global{{Type: GlobalType{Mutable: true}}},
			},
			expectedGlobals: []GlobalType{{Mutable: true}},
		},
		{
			module: &Module{
				ImportSection: []Import{{Type: ExternTypeGlobal, DescGlobal: GlobalType{Mutable: false}}},
			},
			expectedGlobals: []GlobalType{{Mutable: false}},
		},
		// Memories.
		{
			module: &Module{
				ImportSection: []Import{{Type: ExternTypeMemory, DescMem: &Memory{Min: 1, Max: 10}}},
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
				ImportSection: []Import{{Type: ExternTypeTable, DescTable: Table{Min: 1}}},
			},
			expectedTables: []Table{{Min: 1}},
		},
		{
			module: &Module{
				TableSection: []Table{{Min: 10}},
			},
			expectedTables: []Table{{Min: 10}},
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			functions, globals, memory, tables, err := tc.module.AllDeclarations()
			require.NoError(t, err)
			require.Equal(t, tc.expectedFunctions, functions)
			require.Equal(t, tc.expectedGlobals, globals)
			require.Equal(t, tc.expectedTables, tables)
			require.Equal(t, tc.expectedMemory, memory)
		})
	}
}

func TestValidateConstExpression(t *testing.T) {
	t.Run("invalid opcode", func(t *testing.T) {
		expr := ConstantExpression{Opcode: OpcodeNop}
		err := validateConstExpression(nil, 0, &expr, valueTypeUnknown)
		require.Error(t, err)
	})
	for _, vt := range []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64} {
		t.Run(ValueTypeName(vt), func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				expr := ConstantExpression{}
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

				err := validateConstExpression(nil, 0, &expr, vt)
				require.NoError(t, err)
			})
			t.Run("invalid", func(t *testing.T) {
				// Empty data must be failure.
				expr := ConstantExpression{Data: make([]byte, 0)}
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
				err := validateConstExpression(nil, 0, &expr, vt)
				require.Error(t, err)
			})
		})
	}
	t.Run("ref types", func(t *testing.T) {
		t.Run("ref.func", func(t *testing.T) {
			expr := &ConstantExpression{Data: []byte{5}, Opcode: OpcodeRefFunc}
			err := validateConstExpression(nil, 10, expr, ValueTypeFuncref)
			require.NoError(t, err)
			err = validateConstExpression(nil, 2, expr, ValueTypeFuncref)
			require.EqualError(t, err, "ref.func index out of range [5] with length 1")
		})
		t.Run("ref.null", func(t *testing.T) {
			err := validateConstExpression(nil, 0,
				&ConstantExpression{Data: []byte{ValueTypeFuncref}, Opcode: OpcodeRefNull},
				ValueTypeFuncref)
			require.NoError(t, err)
			err = validateConstExpression(nil, 0,
				&ConstantExpression{Data: []byte{ValueTypeExternref}, Opcode: OpcodeRefNull},
				ValueTypeExternref)
			require.NoError(t, err)
			err = validateConstExpression(nil, 0,
				&ConstantExpression{Data: []byte{0xff}, Opcode: OpcodeRefNull},
				ValueTypeExternref)
			require.EqualError(t, err, "invalid type for ref.null: 0xff")
		})
	})
	t.Run("global expr", func(t *testing.T) {
		t.Run("failed to read global index", func(t *testing.T) {
			// Empty data for global index is invalid.
			expr := &ConstantExpression{Data: make([]byte, 0), Opcode: OpcodeGlobalGet}
			err := validateConstExpression(nil, 0, expr, valueTypeUnknown)
			require.Error(t, err)
		})
		t.Run("global index out of range", func(t *testing.T) {
			// Data holds the index in leb128 and this time the value exceeds len(globals) (=0).
			expr := &ConstantExpression{Data: []byte{1}, Opcode: OpcodeGlobalGet}
			var globals []GlobalType
			err := validateConstExpression(globals, 0, expr, valueTypeUnknown)
			require.Error(t, err)
		})

		t.Run("type mismatch", func(t *testing.T) {
			for _, vt := range []ValueType{
				ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64,
			} {
				t.Run(ValueTypeName(vt), func(t *testing.T) {
					// The index specified in Data equals zero.
					expr := &ConstantExpression{Data: []byte{0}, Opcode: OpcodeGlobalGet}
					globals := []GlobalType{{ValType: valueTypeUnknown}}

					err := validateConstExpression(globals, 0, expr, vt)
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
					globals := []GlobalType{{ValType: vt}}

					err := validateConstExpression(globals, 0, expr, vt)
					require.NoError(t, err)
				})
			}
		})
	})
}

func TestModule_Validate_Errors(t *testing.T) {
	zero := Index(0)
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
				CodeSection:     []Code{{Body: []byte{OpcodeEnd}}},
				StartSection:    &zero,
			},
			expectedErr: "invalid start function: func[0] has an invalid type",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.Validate(api.CoreFeaturesV1)
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
		for _, ft := range []FunctionType{
			{Params: []ValueType{ValueTypeI32}},
			{Results: []ValueType{ValueTypeI32}},
			{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI32}},
		} {
			t.Run(ft.String(), func(t *testing.T) {
				index := uint32(0)
				m := Module{StartSection: &index, FunctionSection: []uint32{0}, TypeSection: []FunctionType{ft}}
				err := m.validateStartSection()
				require.Error(t, err)
			})
		}
	})
	t.Run("imported valid func", func(t *testing.T) {
		index := Index(1)
		m := Module{
			StartSection:        &index,
			TypeSection:         []FunctionType{{}, {Results: []ValueType{ValueTypeI32}}},
			ImportFunctionCount: 2,
			ImportSection: []Import{
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
		err := m.validateGlobals(make([]GlobalType, 10), 0, 9)
		require.Error(t, err)
		require.EqualError(t, err, "too many globals in a module")
	})
	t.Run("global index out of range", func(t *testing.T) {
		m := Module{GlobalSection: []Global{
			{
				Type: GlobalType{ValType: ValueTypeI32},
				// Trying to reference globals[1] which is not imported.
				Init: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{1}},
			},
		}}
		err := m.validateGlobals(nil, 0, 9)
		require.Error(t, err)
		require.EqualError(t, err, "global index out of range")
	})
	t.Run("invalid const expression", func(t *testing.T) {
		m := Module{GlobalSection: []Global{
			{
				Type: GlobalType{ValType: valueTypeUnknown},
				Init: ConstantExpression{Opcode: OpcodeUnreachable},
			},
		}}
		err := m.validateGlobals(nil, 0, 9)
		require.Error(t, err)
		require.EqualError(t, err, "invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{GlobalSection: []Global{
			{
				Type: GlobalType{ValType: ValueTypeI32},
				Init: ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
			},
		}}
		err := m.validateGlobals(nil, 0, 9)
		require.NoError(t, err)
	})
	t.Run("ok with imported global", func(t *testing.T) {
		m := Module{
			ImportGlobalCount: 1,
			GlobalSection: []Global{
				{
					Type: GlobalType{ValType: ValueTypeI32},
					// Trying to reference globals[1] which is imported.
					Init: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0}},
				},
			},
			ImportSection: []Import{{Type: ExternTypeGlobal}},
		}
		globalDeclarations := []GlobalType{
			{ValType: ValueTypeI32}, // Imported one.
			{},                      // the local one trying to validate.
		}
		err := m.validateGlobals(globalDeclarations, 0, 9)
		require.NoError(t, err)
	})
}

func TestModule_validateFunctions(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []uint32{0},
			CodeSection:     []Code{{Body: []byte{OpcodeI32Const, 0, OpcodeDrop, OpcodeEnd}}},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.NoError(t, err)
	})
	t.Run("too many functions", func(t *testing.T) {
		m := Module{}
		err := m.validateFunctions(api.CoreFeaturesV1, []uint32{1, 2, 3, 4}, nil, nil, nil, 3)
		require.Error(t, err)
		require.EqualError(t, err, "too many functions (4) in a module")
	})
	t.Run("function, but no code", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection:     nil,
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.EqualError(t, err, "code count (0) != function count (1)")
	})
	t.Run("function out of range of code", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{1},
			CodeSection:     []Code{{Body: []byte{OpcodeEnd}}},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.EqualError(t, err, "invalid function[0]: type section index 1 out of range")
	})
	t.Run("invalid", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection:     []Code{{Body: []byte{OpcodeF32Abs}}},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid function[0]: cannot pop the 1st f32 operand")
	})
	t.Run("in- exported", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection:     []Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection:   []Export{{Name: "f1", Type: ExternTypeFunc, Index: 0}},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1"]: cannot pop the 1st f32`)
	})
	t.Run("in- exported after import", func(t *testing.T) {
		m := Module{
			ImportFunctionCount: 1,
			TypeSection:         []FunctionType{v_v},
			ImportSection:       []Import{{Type: ExternTypeFunc}},
			FunctionSection:     []Index{0},
			CodeSection:         []Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection:       []Export{{Name: "f1", Type: ExternTypeFunc, Index: 1}},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1"]: cannot pop the 1st f32`)
	})
	t.Run("in- exported twice", func(t *testing.T) {
		m := Module{
			TypeSection:     []FunctionType{v_v},
			FunctionSection: []Index{0},
			CodeSection:     []Code{{Body: []byte{OpcodeF32Abs}}},
			ExportSection: []Export{
				{Name: "f1", Type: ExternTypeFunc, Index: 0},
				{Name: "f2", Type: ExternTypeFunc, Index: 0},
			},
		}
		err := m.validateFunctions(api.CoreFeaturesV1, nil, nil, nil, nil, MaximumFunctionIndex)
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid function[0] export["f1","f2"]: cannot pop the 1st f32`)
	})
}

func TestModule_validateMemory(t *testing.T) {
	t.Run("active data segment exits but memory not declared", func(t *testing.T) {
		m := Module{DataSection: []DataSegment{{OffsetExpression: ConstantExpression{}}}}
		err := m.validateMemory(nil, nil, api.CoreFeaturesV1)
		require.Error(t, err)
		require.Contains(t, "unknown memory", err.Error())
	})
	t.Run("invalid const expr", func(t *testing.T) {
		m := Module{DataSection: []DataSegment{{
			OffsetExpression: ConstantExpression{
				Opcode: OpcodeUnreachable, // Invalid!
			},
		}}}
		err := m.validateMemory(&Memory{}, nil, api.CoreFeaturesV1)
		require.EqualError(t, err, "calculate offset: invalid opcode for const expression: 0x0")
	})
	t.Run("ok", func(t *testing.T) {
		m := Module{DataSection: []DataSegment{{
			Init: []byte{0x1},
			OffsetExpression: ConstantExpression{
				Opcode: OpcodeI32Const,
				Data:   leb128.EncodeInt32(1),
			},
		}}}
		err := m.validateMemory(&Memory{}, nil, api.CoreFeaturesV1)
		require.NoError(t, err)
	})
}

func TestModule_validateImports(t *testing.T) {
	tests := []struct {
		name            string
		enabledFeatures api.CoreFeatures
		i               *Import
		expectedErr     string
	}{
		{name: "empty import section"},
		{
			name:            "reject empty named module",
			enabledFeatures: api.CoreFeaturesV1,
			i:               &Import{Module: "", Name: "n", Type: ExternTypeFunc, DescFunc: 0},
			expectedErr:     "import[0] has an empty module name",
		},
		{
			name:            "func",
			enabledFeatures: api.CoreFeaturesV1,
			i:               &Import{Module: "m", Name: "n", Type: ExternTypeFunc, DescFunc: 0},
		},
		{
			name:            "func type index out of range ",
			enabledFeatures: api.CoreFeaturesV1,
			i:               &Import{Module: "m", Name: "n", Type: ExternTypeFunc, DescFunc: 100},
			expectedErr:     "invalid import[\"m\".\"n\"] function: type index out of range",
		},
		{
			name:            "global var disabled",
			enabledFeatures: api.CoreFeaturesV1.SetEnabled(api.CoreFeatureMutableGlobal, false),
			i: &Import{
				Module:     "m",
				Name:       "n",
				Type:       ExternTypeGlobal,
				DescGlobal: GlobalType{ValType: ValueTypeI32, Mutable: true},
			},
			expectedErr: `invalid import["m"."n"] global: feature "mutable-global" is disabled`,
		},
		{
			name:            "table",
			enabledFeatures: api.CoreFeaturesV1,
			i: &Import{
				Module:    "m",
				Name:      "n",
				Type:      ExternTypeTable,
				DescTable: Table{Min: 1},
			},
		},
		{
			name:            "memory",
			enabledFeatures: api.CoreFeaturesV1,
			i: &Import{
				Module:  "m",
				Name:    "n",
				Type:    ExternTypeMemory,
				DescMem: &Memory{Min: 1},
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := Module{TypeSection: []FunctionType{{}}}
			if tc.i != nil {
				m.ImportSection = []Import{*tc.i}
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
	tests := []struct {
		name            string
		enabledFeatures api.CoreFeatures
		exportSection   []Export
		functions       []Index
		globals         []GlobalType
		memory          *Memory
		tables          []Table
		expectedErr     string
	}{
		{name: "empty export section", exportSection: []Export{}},
		{
			name:            "func",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeFunc, Index: 0}},
			functions:       []Index{100 /* arbitrary type id*/},
		},
		{
			name:            "func out of range",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeFunc, Index: 1, Name: "e"}},
			functions:       []Index{100 /* arbitrary type id*/},
			expectedErr:     `unknown function for export["e"]`,
		},
		{
			name:            "global const",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeGlobal, Index: 0}},
			globals:         []GlobalType{{ValType: ValueTypeI32}},
		},
		{
			name:            "global var",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeGlobal, Index: 0}},
			globals:         []GlobalType{{ValType: ValueTypeI32, Mutable: true}},
		},
		{
			name:            "global var disabled",
			enabledFeatures: api.CoreFeaturesV1.SetEnabled(api.CoreFeatureMutableGlobal, false),
			exportSection:   []Export{{Type: ExternTypeGlobal, Index: 0, Name: "e"}},
			globals:         []GlobalType{{ValType: ValueTypeI32, Mutable: true}},
			expectedErr:     `invalid export["e"] global[0]: feature "mutable-global" is disabled`,
		},
		{
			name:            "global out of range",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeGlobal, Index: 1, Name: "e"}},
			globals:         []GlobalType{{}},
			expectedErr:     `unknown global for export["e"]`,
		},
		{
			name:            "table",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeTable, Index: 0}},
			tables:          []Table{{}},
		},
		{
			name:            "multiple tables",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeTable, Index: 0}, {Type: ExternTypeTable, Index: 1}, {Type: ExternTypeTable, Index: 2}},
			tables:          []Table{{}, {}, {}},
		},
		{
			name:            "table out of range",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeTable, Index: 1, Name: "e"}},
			tables:          []Table{},
			expectedErr:     `table for export["e"] out of range`,
		},
		{
			name:            "memory",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeMemory, Index: 0}},
			memory:          &Memory{},
		},
		{
			name:            "memory out of range",
			enabledFeatures: api.CoreFeaturesV1,
			exportSection:   []Export{{Type: ExternTypeMemory, Index: 0, Name: "e"}},
			tables:          []Table{},
			expectedErr:     `memory for export["e"] out of range`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := Module{ExportSection: tc.exportSection}
			err := m.validateExports(tc.enabledFeatures, tc.functions, tc.globals, tc.memory, tc.tables)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModule_buildGlobals(t *testing.T) {
	const localFuncRefInstructionIndex = uint32(0xffff)

	minusOne := int32(-1)
	m := &Module{
		ImportGlobalCount: 2,
		GlobalSection: []Global{
			{
				Type: GlobalType{Mutable: true, ValType: ValueTypeF64},
				Init: ConstantExpression{
					Opcode: OpcodeF64Const,
					Data:   u64.LeBytes(api.EncodeF64(math.MaxFloat64)),
				},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeI32},
				Init: ConstantExpression{
					Opcode: OpcodeI32Const,
					Data:   leb128.EncodeInt32(math.MaxInt32),
				},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeI32},
				Init: ConstantExpression{
					Opcode: OpcodeI32Const,
					Data:   leb128.EncodeInt32(minusOne),
				},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeV128},
				Init: ConstantExpression{
					Opcode: OpcodeVecV128Const,
					Data: []byte{
						1, 0, 0, 0, 0, 0, 0, 0,
						2, 0, 0, 0, 0, 0, 0, 0,
					},
				},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeExternref},
				Init: ConstantExpression{Opcode: OpcodeRefNull, Data: []byte{ValueTypeExternref}},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeFuncref},
				Init: ConstantExpression{Opcode: OpcodeRefNull, Data: []byte{ValueTypeFuncref}},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeFuncref},
				Init: ConstantExpression{Opcode: OpcodeRefFunc, Data: leb128.EncodeUint32(localFuncRefInstructionIndex)},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeExternref},
				Init: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0}},
			},
			{
				Type: GlobalType{Mutable: false, ValType: ValueTypeFuncref},
				Init: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{1}},
			},
		},
	}

	imported := []*GlobalInstance{
		{Type: GlobalType{ValType: ValueTypeExternref}, Val: 0x54321},
		{Type: GlobalType{ValType: ValueTypeFuncref}, Val: 0x12345},
	}

	mi := &ModuleInstance{
		Globals: make([]*GlobalInstance, m.ImportGlobalCount+uint32(len(m.GlobalSection))),
		Engine:  &mockModuleEngine{},
	}

	mi.Globals[0], mi.Globals[1] = imported[0], imported[1]

	mi.buildGlobals(m, func(funcIndex Index) Reference {
		require.Equal(t, localFuncRefInstructionIndex, funcIndex)
		return 0x99999
	})
	expectedGlobals := []*GlobalInstance{
		imported[0], imported[1],
		{Type: GlobalType{ValType: ValueTypeF64, Mutable: true}, Val: api.EncodeF64(math.MaxFloat64)},
		{Type: GlobalType{ValType: ValueTypeI32, Mutable: false}, Val: uint64(int32(math.MaxInt32))},
		// Higher bits are must be zeroed for i32 globals, not signed-extended. See #656.
		{Type: GlobalType{ValType: ValueTypeI32, Mutable: false}, Val: uint64(uint32(minusOne))},
		{Type: GlobalType{ValType: ValueTypeV128, Mutable: false}, Val: 0x1, ValHi: 0x2},
		{Type: GlobalType{ValType: ValueTypeExternref, Mutable: false}, Val: 0},
		{Type: GlobalType{ValType: ValueTypeFuncref, Mutable: false}, Val: 0},
		{Type: GlobalType{ValType: ValueTypeFuncref, Mutable: false}, Val: 0x99999},
		{Type: GlobalType{ValType: ValueTypeExternref, Mutable: false}, Val: 0x54321},
		{Type: GlobalType{ValType: ValueTypeFuncref, Mutable: false}, Val: 0x12345},
	}
	require.Equal(t, expectedGlobals, mi.Globals)
}

func TestModule_buildMemoryInstance(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		m := ModuleInstance{}
		m.buildMemory(&Module{}, nil)
		require.Nil(t, m.MemoryInstance)
	})
	t.Run("non-nil", func(t *testing.T) {
		min := uint32(1)
		max := uint32(10)
		mDef := MemoryDefinition{moduleName: "foo"}
		m := ModuleInstance{}
		m.buildMemory(&Module{
			MemorySection:           &Memory{Min: min, Cap: min, Max: max},
			MemoryDefinitionSection: []MemoryDefinition{mDef},
		}, nil)
		mem := m.MemoryInstance
		require.Equal(t, min, mem.Min)
		require.Equal(t, max, mem.Max)
		require.Equal(t, &mDef, mem.definition)
	})
}

func TestModule_validateDataCountSection(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		for _, m := range []*Module{
			{
				DataSection:      []DataSegment{},
				DataCountSection: nil,
			},
			{
				DataSection:      []DataSegment{{}, {}},
				DataCountSection: nil,
			},
		} {
			err := m.validateDataCountSection()
			require.NoError(t, err)
		}
	})
	t.Run("error", func(t *testing.T) {
		count := uint32(1)
		for _, m := range []*Module{
			{
				DataSection:      []DataSegment{},
				DataCountSection: &count,
			},
			{
				DataSection:      []DataSegment{{}, {}},
				DataCountSection: &count,
			},
		} {
			err := m.validateDataCountSection()
			require.Error(t, err)
		}
	})
}

func TestModule_declaredFunctionIndexes(t *testing.T) {
	tests := []struct {
		name   string
		mod    *Module
		exp    map[Index]struct{}
		expErr string
	}{
		{
			name: "empty",
			mod:  &Module{},
			exp:  map[uint32]struct{}{},
		},
		{
			name: "global",
			mod: &Module{
				ExportSection: []Export{
					{Index: 10, Type: ExternTypeFunc},
					{Index: 1000, Type: ExternTypeGlobal},
				},
			},
			exp: map[uint32]struct{}{10: {}},
		},
		{
			name: "export",
			mod: &Module{
				ExportSection: []Export{
					{Index: 1000, Type: ExternTypeGlobal},
					{Index: 10, Type: ExternTypeFunc},
				},
			},
			exp: map[uint32]struct{}{10: {}},
		},
		{
			name: "element",
			mod: &Module{
				ElementSection: []ElementSegment{
					{
						Mode: ElementModeActive,
						Init: []Index{0, ElementInitNullReference, 5},
					},
					{
						Mode: ElementModeDeclarative,
						Init: []Index{1, ElementInitNullReference, 5},
					},
					{
						Mode: ElementModePassive,
						Init: []Index{5, 2, ElementInitNullReference, ElementInitNullReference},
					},
				},
			},
			exp: map[uint32]struct{}{0: {}, 1: {}, 2: {}, 5: {}},
		},
		{
			name: "all",
			mod: &Module{
				ExportSection: []Export{
					{Index: 10, Type: ExternTypeGlobal},
					{Index: 1000, Type: ExternTypeFunc},
				},
				GlobalSection: []Global{
					{
						Init: ConstantExpression{
							Opcode: OpcodeI32Const, // not funcref.
							Data:   leb128.EncodeInt32(-1),
						},
					},
					{
						Init: ConstantExpression{
							Opcode: OpcodeRefFunc,
							Data:   leb128.EncodeInt32(123),
						},
					},
				},
				ElementSection: []ElementSegment{
					{
						Mode: ElementModeActive,
						Init: []Index{0, ElementInitNullReference, 5},
					},
					{
						Mode: ElementModeDeclarative,
						Init: []Index{1, ElementInitNullReference, 5},
					},
					{
						Mode: ElementModePassive,
						Init: []Index{5, 2, ElementInitNullReference, ElementInitNullReference},
					},
				},
			},
			exp: map[uint32]struct{}{0: {}, 1: {}, 2: {}, 5: {}, 123: {}, 1000: {}},
		},
		{
			mod: &Module{
				GlobalSection: []Global{
					{
						Init: ConstantExpression{
							Opcode: OpcodeRefFunc,
							Data:   nil,
						},
					},
				},
			},
			name:   "invalid global",
			expErr: `global[0] failed to initialize: EOF`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.mod.declaredFunctionIndexes()
			if tc.expErr != "" {
				require.EqualError(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.exp, actual)
			}
		})
	}
}

func TestModule_AssignModuleID(t *testing.T) {
	getID := func(bin []byte, lsns []experimental.FunctionListener, withEnsureTermination bool) ModuleID {
		m := Module{}
		m.AssignModuleID(bin, lsns, withEnsureTermination)
		return m.ID
	}

	ml := &mockListener{}

	// Ensures that different args always produce the different IDs.
	exists := map[ModuleID]struct{}{}
	for i, tc := range []struct {
		bin                   []byte
		withEnsureTermination bool
		listeners             []experimental.FunctionListener
	}{
		{bin: []byte{1, 2, 3}, withEnsureTermination: false},
		{bin: []byte{1, 2, 3}, withEnsureTermination: true},
		{
			bin:                   []byte{1, 2, 3},
			listeners:             []experimental.FunctionListener{ml},
			withEnsureTermination: false,
		},
		{
			bin:                   []byte{1, 2, 3},
			listeners:             []experimental.FunctionListener{ml},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3},
			listeners:             []experimental.FunctionListener{nil, ml},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3},
			listeners:             []experimental.FunctionListener{ml, ml},
			withEnsureTermination: true,
		},
		{bin: []byte{1, 2, 3, 4}, withEnsureTermination: false},
		{bin: []byte{1, 2, 3, 4}, withEnsureTermination: true},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{ml},
			withEnsureTermination: false,
		},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{ml},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{nil},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{nil, ml},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{ml, ml},
			withEnsureTermination: true,
		},
		{
			bin:                   []byte{1, 2, 3, 4},
			listeners:             []experimental.FunctionListener{ml, ml},
			withEnsureTermination: false,
		},
	} {
		id := getID(tc.bin, tc.listeners, tc.withEnsureTermination)
		_, exist := exists[id]
		require.False(t, exist, i)
		exists[id] = struct{}{}
	}
}

type mockListener struct{}

func (m mockListener) Before(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator) {
}

func (m mockListener) After(context.Context, api.Module, api.FunctionDefinition, []uint64) {}

func (m mockListener) Abort(context.Context, api.Module, api.FunctionDefinition, error) {}
