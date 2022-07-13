package wasm

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func Test_resolveImports_table(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("ok", func(t *testing.T) {
		max := uint32(10)
		tableInst := &TableInstance{Max: &max}
		modules := map[string]*ModuleInstance{
			moduleName: {Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: tableInst,
			}}, Name: moduleName},
		}
		_, _, tables, _, err := resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: &Table{Max: &max}}}}, modules)
		require.NoError(t, err)
		require.Equal(t, 1, len(tables))
		require.Equal(t, tables[0], tableInst)
	})
	t.Run("minimum size mismatch", func(t *testing.T) {
		importTableType := &Table{Min: 2}
		modules := map[string]*ModuleInstance{
			moduleName: {Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Min - 1},
			}}, Name: moduleName},
		}
		_, _, _, _, err := resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}}, modules)
		require.EqualError(t, err, "import[0] table[test.target]: minimum size mismatch: 2 > 1")
	})
	t.Run("maximum size mismatch", func(t *testing.T) {
		max := uint32(10)
		importTableType := &Table{Max: &max}
		modules := map[string]*ModuleInstance{
			moduleName: {Exports: map[string]*ExportInstance{name: {
				Type:  ExternTypeTable,
				Table: &TableInstance{Min: importTableType.Min - 1},
			}}, Name: moduleName},
		}
		_, _, _, _, err := resolveImports(&Module{ImportSection: []*Import{{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}}}, modules)
		require.EqualError(t, err, "import[0] table[test.target]: maximum size mismatch: 10, but actual has no max")
	})
}

var codeEnd = &Code{Body: []byte{OpcodeEnd}}

func TestModule_validateTable(t *testing.T) {
	const maxTableIndex = 5
	three := uint32(3)
	tests := []struct {
		name     string
		input    *Module
		expected []*validatedActiveElementSegment
	}{
		{
			name:     "empty",
			input:    &Module{},
			expected: []*validatedActiveElementSegment{},
		},
		{
			name:     "min zero",
			input:    &Module{TableSection: []*Table{{}}},
			expected: []*validatedActiveElementSegment{},
		},
		{
			name:     "maximum number of tables",
			input:    &Module{TableSection: []*Table{{}, {}, {}, {}, {}}},
			expected: []*validatedActiveElementSegment{},
		},
		{
			name:     "min/max",
			input:    &Module{TableSection: []*Table{{Min: 1, Max: &three}}},
			expected: []*validatedActiveElementSegment{},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Type: RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{},
		},
		{
			name: "constant derived element offset=0 and one index",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
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
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
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
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
			},
		},
		{
			name: "constant derived element offset and two indices",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
						Init:       []*Index{uint32Ptr(0), uint32Ptr(2)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeI32Const, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
			},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Type: RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{},
		},
		{
			name: "imported global derived element offset and one index",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
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
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
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
						Init:       []*Index{uint32Ptr(0)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
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
				TableSection:    []*Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []*Index{uint32Ptr(0), uint32Ptr(2)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeGlobalGet, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
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
				TableSection:    []*Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
						Init:       []*Index{uint32Ptr(0), uint32Ptr(2)},
						Type:       RefTypeFuncref,
					},
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []*Index{uint32Ptr(1), uint32Ptr(2)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expected: []*validatedActiveElementSegment{
				{opcode: OpcodeI32Const, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
				{opcode: OpcodeGlobalGet, arg: 1, init: []*Index{uint32Ptr(1), uint32Ptr(2)}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, _, tables, err := tc.input.AllDeclarations()
			require.NoError(t, err)

			vt, err := tc.input.validateTable(Features20191205, tables, maxTableIndex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, vt)

			// Ensure it was cached. We have to use Equal not Same because this is a slice, not a pointer.
			require.Equal(t, vt, tc.input.validatedActiveElementSegments)
			vt2, err := tc.input.validateTable(Features20191205, tables, maxTableIndex)
			require.NoError(t, err)
			require.Equal(t, vt, vt2)
		})
	}
}

func TestModule_validateTable_Errors(t *testing.T) {
	const maxTableIndex = 5
	tests := []struct {
		name        string
		input       *Module
		expectedErr string
	}{
		{
			name: "too many tables",
			input: &Module{
				TableSection: []*Table{{}, {}, {}, {}, {}, {}},
			},
			expectedErr: "too many tables in a module: 6 given with limit 5",
		},
		{
			name: "unknown ref type",
			input: &Module{
				TableSection: []*Table{{}},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Type: 0xff,
					},
				},
			},
			expectedErr: "only funcref or externref element can be used to initialize table, but was unknown(0xff)",
		},
		{
			name: "constant derived element offset - decode error",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] couldn't read i32.const parameter: overflows a 32-bit integer",
		},
		{
			name: "constant derived element offset - wrong ValType",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI64Const, Data: const0}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
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
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "unknown table 0 as active element target",
		},
		{
			name: "constant derived element offset exceeds table min",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset puts init beyond table min",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []*Index{uint32Ptr(0), uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[1].init exceeds min table size",
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset beyond table min - no init elements",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset - funcidx out of range",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []*Index{uint32Ptr(0), uint32Ptr(1)},
						Type: RefTypeFuncref,
					},
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
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "unknown table 0 as active element target",
		},
		{
			name: "imported global derived element offset - funcidx out of range",
			input: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []*Index{uint32Ptr(0), uint32Ptr(1)},
						Type: RefTypeFuncref,
					},
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
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
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
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{
							Opcode: OpcodeGlobalGet,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] couldn't read global.get parameter: overflows a 32-bit integer",
		},
		{
			name: "imported global derived element offset - no imports",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				GlobalSection:   []*Global{{Type: &GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
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
				TableSection:    []*Table{{}},
				FunctionSection: []Index{0},
				GlobalSection:   []*Global{{Type: &GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []*Index{uint32Ptr(0)},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] (global.get 0): out of range of imported globals",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, _, tables, err := tc.input.AllDeclarations()
			require.NoError(t, err)
			_, err = tc.input.validateTable(Features20191205, tables, maxTableIndex)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

var const0 = leb128.EncodeInt32(0)
var const1 = leb128.EncodeInt32(1)

func TestModule_buildTables(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name            string
		module          *Module
		importedTables  []*TableInstance
		importedGlobals []*GlobalInstance
		expectedTables  []*TableInstance
		expectedInit    []TableInitEntry
	}{
		{
			name: "empty",
			module: &Module{
				validatedActiveElementSegments: []*validatedActiveElementSegment{},
			},
		},
		{
			name: "min zero",
			module: &Module{
				TableSection:                   []*Table{{Type: RefTypeFuncref}},
				validatedActiveElementSegments: []*validatedActiveElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 0), Min: 0, Type: RefTypeFuncref}},
		},
		{
			name: "min/max",
			module: &Module{
				TableSection:                   []*Table{{Min: 1, Max: &three}},
				validatedActiveElementSegments: []*validatedActiveElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1, Max: &three}},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			module: &Module{
				TypeSection:                    []*FunctionType{{}},
				TableSection:                   []*Table{{Min: 1}},
				FunctionSection:                []Index{0},
				CodeSection:                    []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1}},
		},
		{
			name: "constant derived element offset=0 and one index",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1}},
			expectedInit:   []TableInitEntry{{TableIndex: 0, Offset: 0, FunctionIndexes: []*Index{uint32Ptr(0)}}},
		},
		{
			name: "constant derived element offset - imported table",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedTables: []*TableInstance{{Min: 2}},
			expectedTables: []*TableInstance{{Min: 2}},
			expectedInit:   []TableInitEntry{{TableIndex: 0, Offset: 0, FunctionIndexes: []*Index{uint32Ptr(0)}}},
		},
		{
			name: "constant derived element offset=0 and one index - imported table",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				ImportSection:   []*Import{{Type: ExternTypeTable, DescTable: &Table{Min: 1}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedTables: []*TableInstance{{Min: 1}},
			expectedTables: []*TableInstance{{Min: 1}},
			expectedInit:   []TableInitEntry{{TableIndex: 0, Offset: 0, FunctionIndexes: []*Index{uint32Ptr(0)}}},
		},
		{
			name: "constant derived element offset and two indices",
			module: &Module{
				TypeSection:     []*FunctionType{{}},
				TableSection:    []*Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
				},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 3), Min: 3}},
			expectedInit:   []TableInitEntry{{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0), uint32Ptr(2)}}},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:                   []*Table{{Min: 1}},
				FunctionSection:                []Index{0},
				CodeSection:                    []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTables:  []*TableInstance{{References: make([]Reference, 1), Min: 1}},
		},
		{
			name: "imported global derived element offset and one index",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedInit:    []TableInitEntry{{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0)}}},
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
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedTables:  []*TableInstance{{Min: 2, References: []Reference{0, 0}}},
			expectedInit:    []TableInitEntry{{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0)}}},
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
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedTables:  []*TableInstance{{Min: 2, References: []Reference{0, 0}}},
			expectedInit:    []TableInitEntry{{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0)}}},
		},
		{
			name: "imported global derived element offset - two indices",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 3}, {Min: 100}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []*Index{nil, uint32Ptr(2)},
						TableIndex: 1,
					},
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []*Index{uint32Ptr(0), uint32Ptr(2)},
						TableIndex: 0,
					},
				},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{tableIndex: 1, opcode: OpcodeGlobalGet, arg: 0, init: []*Index{nil, uint32Ptr(2)}},
					{tableIndex: 0, opcode: OpcodeGlobalGet, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: &GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTables: []*TableInstance{
				{References: make([]Reference, 3), Min: 3},
				{References: make([]Reference, 100), Min: 100},
			},
			expectedInit: []TableInitEntry{
				{TableIndex: 1, Offset: 3, FunctionIndexes: []*Index{nil, uint32Ptr(2)}},
				{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0), uint32Ptr(2)}},
			},
		},
		{
			name: "mixed elementSegments - const before imported global",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []*Code{codeEnd, codeEnd, codeEnd, codeEnd},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 1, init: []*Index{uint32Ptr(0), uint32Ptr(2)}},
					{opcode: OpcodeGlobalGet, arg: 1, init: []*Index{uint32Ptr(1), uint32Ptr(2)}},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: &GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: &GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 3), Min: 3}},
			expectedInit: []TableInitEntry{
				{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(0), uint32Ptr(2)}},
				{TableIndex: 0, Offset: 1, FunctionIndexes: []*Index{uint32Ptr(1), uint32Ptr(2)}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			tables, init, err := tc.module.buildTables(tc.importedTables, tc.importedGlobals, false)
			require.NoError(t, err)

			require.Equal(t, tc.expectedTables, tables)
			require.Equal(t, tc.expectedInit, init)
		})
	}
}

// TestModule_buildTable_Errors covers the only late error conditions possible.
func TestModule_buildTable_Errors(t *testing.T) {
	tests := []struct {
		name            string
		module          *Module
		importedTables  []*TableInstance
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
						OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []*Index{uint32Ptr(0)},
					},
				},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeI32Const, arg: 2, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedTables: []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedErr:    "element[0].init exceeds min table size",
		},
		{
			name: "imported global derived element offset exceeds table min",
			module: &Module{
				TypeSection: []*FunctionType{{}},
				ImportSection: []*Import{
					{Type: ExternTypeGlobal, DescGlobal: &GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []*Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []*Index{uint32Ptr(0)},
					},
				},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
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
				TableSection:    []*Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{codeEnd},
				ElementSection: []*ElementSegment{
					{
						OffsetExpr: &ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []*Index{uint32Ptr(0)},
					},
				},
				validatedActiveElementSegments: []*validatedActiveElementSegment{
					{opcode: OpcodeGlobalGet, arg: 0, init: []*Index{uint32Ptr(0)}},
				},
			},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			importedGlobals: []*GlobalInstance{{Type: &GlobalType{ValType: ValueTypeI32}, Val: 2}},
			expectedErr:     "element[0].init exceeds min table size",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.module.buildTables(tc.importedTables, tc.importedGlobals, false)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestTableInstance_Grow(t *testing.T) {
	expOnErr := uint32(0xffff_ffff) // -1 as signed i32.
	max10 := uint32(10)
	tests := []struct {
		name       string
		currentLen int
		max        *uint32
		delta, exp uint32
	}{
		{
			name:       "growing ousside 32-bit range",
			currentLen: 0x10,
			delta:      0xffff_fff0,
			exp:        expOnErr,
		},
		{
			name:       "growing zero",
			currentLen: 0,
			delta:      0,
			exp:        0,
		},
		{
			name:       "growing zero on non zero table",
			currentLen: 5,
			delta:      0,
			exp:        5,
		},
		{
			name:       "grow zero on max",
			currentLen: 10,
			delta:      0,
			max:        &max10,
			exp:        10,
		},
		{
			name:       "grow out of range beyond max",
			currentLen: 10,
			delta:      1,
			max:        &max10,
			exp:        expOnErr,
		},
		{
			name:       "grow out of range beyond max part2",
			currentLen: 10,
			delta:      100,
			max:        &max10,
			exp:        expOnErr,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			table := &TableInstance{References: make([]uintptr, tc.currentLen), Max: tc.max}
			actual := table.Grow(testCtx, tc.delta, 0)
			require.Equal(t, tc.exp, actual)
		})
	}
}
