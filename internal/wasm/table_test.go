package internalwasm

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/leb128"
)

func TestStore_resolveImports_table(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("ok", func(t *testing.T) {
		s := newStore()
		max := uint32(10)
		tableInst := &TableInstance{Max: &max}
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
			Type:  ExternTypeTable,
			Table: tableInst,
		}}, Name: moduleName}
		_, _, table, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &Table{Max: &max}}}})
		require.NoError(t, err)
		require.Equal(t, table, tableInst)
	})
	t.Run("minimum size mismatch", func(t *testing.T) {
		s := newStore()
		importTableType := &Table{Min: 2}
		s.modules[moduleName] = &ModuleInstance{Exports: map[string]*ExportInstance{name: {
			Type:  ExternTypeTable,
			Table: &TableInstance{Min: importTableType.Min - 1},
		}}, Name: moduleName}
		_, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
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
		_, _, _, _, err := s.resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}})
		require.EqualError(t, err, "import[0] table[test.target]: maximum size mismatch: 10, but actual has no max")
	})
}

var codeEnd = &Code{Body: []byte{OpcodeEnd}}

func TestModule_validateTable(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name     string
		input    *Module
		expected []*validatedElementSegment
	}{
		{
			name:     "empty",
			input:    &Module{},
			expected: []*validatedElementSegment{},
		},
		{
			name:     "min zero",
			input:    &Module{TableSection: &Table{}},
			expected: []*validatedElementSegment{},
		},
		{
			name:     "min/max",
			input:    &Module{TableSection: &Table{1, &three}},
			expected: []*validatedElementSegment{},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}},
				},
			},
			expected: []*validatedElementSegment{},
		},
		{
			name: "constant derived element offset=0 and one index",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "constant derived element offset - ignores min on imported table",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "constant derived element offset=0 and one index - imported table",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{Min: 1}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "constant derived element offset and two indices",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeI32Const, arg0: 1, init: []uint32{0, 2}},
			},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}},
				},
			},
			expected: []*validatedElementSegment{},
		},
		{
			name: "imported global derived element offset and one index",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "imported global derived element offset and one index - imported table",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeTable, DescTable: &Table{Min: 1}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "imported global derived element offset - ignores min on imported table",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeTable, DescTable: &Table{}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
			},
		},
		{
			name: "imported global derived element offset - two indices",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeGlobalGet, arg0: 1, init: []uint32{0, 2}},
			},
		},
		{
			name: "mixed elementSegments - const before imported global",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{1, 2},
					},
				},
			},
			expected: []*validatedElementSegment{
				{opcode: OpcodeI32Const, arg0: 1, init: []uint32{0, 2}},
				{opcode: OpcodeGlobalGet, arg0: 1, init: []uint32{1, 2}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			vt, err := tc.input.validateTable()
			require.NoError(t, err)
			require.Equal(t, tc.expected, vt)

			// Ensure it was cached. We have to use Equal not Same because this is a slice, not a pointer.
			require.Equal(t, vt, tc.input.validatedElementSegments)
			vt2, err := tc.input.validateTable()
			require.NoError(t, err)
			require.Equal(t, vt, vt2)
		})
	}
}

func TestModule_validateTable_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       *Module
		expectedErr string
	}{
		{
			name: "constant derived element offset - decode error",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{
						Opcode: OpcodeI32Const,
						Data:   leb128.EncodeUint64(math.MaxUint64),
					}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] couldn't read i32.const parameter: overflows a 32-bit integer",
		},
		{
			name: "constant derived element offset - wrong ValType",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI64Const, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] has an invalid const expression: i64.const",
		},
		{
			name: "constant derived element offset - missing table",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element was defined, but not table",
		},
		{
			name: "constant derived element offset exceeds table min",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x2}}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset puts init beyond table min",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 2},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}, Init: []Index{0}},
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}, Init: []Index{0, 0}},
				},
			},
			expectedErr: "element[1].init exceeds min table size",
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset beyond table min - no init elements",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x2}}},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset - funcidx out of range",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}, Init: []Index{0, 1}},
				},
			},
			expectedErr: "element[0].init[1] funcidx 1 out of range",
		},
		{
			name: "imported global derived element offset - missing table",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element was defined, but not table",
		},
		{
			name: "imported global derived element offset - funcidx out of range",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0, 1}},
				},
			},
			expectedErr: "element[0].init[1] funcidx 1 out of range",
		},
		{
			name: "imported global derived element offset - wrong ValType",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
				},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] (global.get 0): import[0].global.ValType != i32",
		},
		{
			name: "imported global derived element offset - decode error",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{
						Opcode: OpcodeGlobalGet,
						Data:   leb128.EncodeUint64(math.MaxUint64),
					}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] couldn't read global.get parameter: overflows a 32-bit integer",
		},
		{
			name: "imported global derived element offset - no imports",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				GlobalSection:   []*Global{{Type: &GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] (global.get 0): out of range of imported globals",
		},
		{
			name: "imported global derived element offset - no imports are globals",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeFunc, DescFunc: 0},
				},
				TableSection:    &Table{},
				FunctionSection: []Index{0},
				GlobalSection:   []*Global{{Type: &GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0}},
				},
			},
			expectedErr: "element[0] (global.get 0): out of range of imported globals",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.input.validateTable()
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModule_buildTable(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name            string
		module          *Module
		importedTable   *TableInstance
		importedGlobals []*GlobalInstance
		expectedTable   *TableInstance
		expectedInit    map[Index]Index
	}{
		{
			name: "empty",
			module: &Module{
				validatedElementSegments: []*validatedElementSegment{},
			},
		},
		{
			name: "min zero",
			module: &Module{
				TableSection:             &Table{},
				validatedElementSegments: []*validatedElementSegment{},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 0), Min: 0},
		},
		{
			name: "min/max",
			module: &Module{
				TableSection:             &Table{1, &three},
				validatedElementSegments: []*validatedElementSegment{},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 1), Min: 1, Max: &three},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}},
				},
				validatedElementSegments: []*validatedElementSegment{},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 1), Min: 1},
		},
		{
			name: "constant derived element offset=0 and one index",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
				},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 1), Min: 1},
			expectedInit:  map[Index]Index{0: 0},
		},
		{
			name: "constant derived element offset - ignores min on imported table",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
				},
			},
		},
		{
			name: "constant derived element offset=0 and one index - imported table",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{Min: 1}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 0, init: []uint32{0}},
				},
			},
		},
		{
			name: "constant derived element offset and two indices",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 1, init: []uint32{0, 2}},
				},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 3), Min: 3},
			expectedInit:  map[Index]Index{1: 0, 2: 2},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 1},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}},
				},
				validatedElementSegments: []*validatedElementSegment{},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTable:   &TableInstance{Table: make([]interface{}, 1), Min: 1},
		},
		{
			name: "imported global derived element offset and one index",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 2},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTable:   &TableInstance{Table: make([]interface{}, 2), Min: 2},
			expectedInit:    map[Index]Index{1: 0},
		},
		{
			name: "imported global derived element offset and one index - imported table",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeTable, DescTable: &Table{Min: 1}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTable:   &TableInstance{Table: make([]interface{}, 2), Min: 2},
			expectedInit:    map[Index]Index{1: 0},
		},
		{
			name: "imported global derived element offset - ignores min on imported table",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeTable, DescTable: &Table{}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTable:   &TableInstance{Table: make([]interface{}, 2), Min: 2},
			expectedInit:    map[Index]Index{1: 0},
		},
		{
			name: "imported global derived element offset - two indices",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 1, init: []uint32{0, 2}},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: &GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 3), Min: 3},
			expectedInit:  map[Index]Index{1: 0, 2: 2},
		},
		{
			name: "mixed elementSegments - const before imported global",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 3},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}},
						Init:       []Index{0, 2},
					},
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{1, 2},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 1, init: []uint32{0, 2}},
					{opcode: OpcodeGlobalGet, arg0: 1, init: []uint32{1, 2}},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: &GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTable: &TableInstance{Table: make([]interface{}, 3), Min: 3},
			expectedInit:  map[Index]Index{1: 1, 2: 2},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			table, init, err := tc.module.buildTable(tc.importedTable, tc.importedGlobals)
			require.NoError(t, err)
			if tc.importedTable != nil { // buildTable shouldn't touch the imported one
				require.Same(t, tc.importedTable, table)
			} else {
				require.Equal(t, tc.expectedTable, table)
			}
			require.Equal(t, tc.expectedInit, init)
		})
	}
}

// TestModule_buildTable_Errors covers the only late error conditions possible.
func TestModule_buildTable_Errors(t *testing.T) {
	tests := []struct {
		name            string
		module          *Module
		importedTable   *TableInstance
		importedGlobals []*GlobalInstance
		expectedErr     string
	}{
		{
			name: "constant derived element offset exceeds table min - imported table",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeI32Const, arg0: 2, init: []uint32{0}},
				},
			},
			importedTable: &TableInstance{Table: make([]interface{}, 2), Min: 2},
			expectedErr:   "element[0].init exceeds min table size",
		},
		{
			name: "imported global derived element offset exceeds table min",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 2},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 2}},
			expectedErr:     "element[0].init exceeds min table size",
		},
		{
			name: "imported global derived element offset exceeds table min imported table",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeTable, DescTable: &Table{}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    &Table{Min: 2},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
				validatedElementSegments: []*validatedElementSegment{
					{opcode: OpcodeGlobalGet, arg0: 0, init: []uint32{0}},
				},
			},
			importedTable:   &TableInstance{Table: make([]interface{}, 2), Min: 2},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 2}},
			expectedErr:     "element[0].init exceeds min table size",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.module.buildTable(tc.importedTable, tc.importedGlobals)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
