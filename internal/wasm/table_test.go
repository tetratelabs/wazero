package wasm

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_ElementInitNullReference_valid ensures it is actually safe to use ElementInitNullReference
// as a null reference, and it won't collide with the actual function Index.
func Test_ElementInitNullReference_valid(t *testing.T) {
	require.True(t, MaximumFunctionIndex < ElementInitNullReference)
}

func Test_resolveImports_table(t *testing.T) {
	const moduleName = "test"
	const name = "target"

	t.Run("ok", func(t *testing.T) {
		max := uint32(10)
		tableInst := &TableInstance{Max: &max, involvingModuleInstances: []*ModuleInstance{{}}}
		s := newStore()
		s.nameToModule[moduleName] = &ModuleInstance{
			Tables:     []*TableInstance{tableInst},
			Exports:    map[string]*Export{name: {Type: ExternTypeTable, Index: 0}},
			ModuleName: moduleName,
		}
		m := &ModuleInstance{Tables: make([]*TableInstance, 1), s: s}
		err := m.resolveImports(context.Background(), &Module{
			ImportPerModule: map[string][]*Import{
				moduleName: {{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: Table{Max: &max}}},
			},
		})
		require.NoError(t, err)
		require.Equal(t, m.Tables[0], tableInst)
		require.Equal(t, m.Tables[0].involvingModuleInstances[1], m)
	})
	t.Run("minimum size mismatch", func(t *testing.T) {
		s := newStore()
		importTableType := Table{Min: 2}
		s.nameToModule[moduleName] = &ModuleInstance{
			Tables:     []*TableInstance{{Min: importTableType.Min - 1}},
			Exports:    map[string]*Export{name: {Type: ExternTypeTable}},
			ModuleName: moduleName,
		}
		m := &ModuleInstance{Tables: make([]*TableInstance, 1), s: s}
		err := m.resolveImports(context.Background(), &Module{
			ImportPerModule: map[string][]*Import{
				moduleName: {{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}},
			},
		})
		require.EqualError(t, err, "import table[test.target]: minimum size mismatch: 2 > 1")
	})
	t.Run("maximum size mismatch", func(t *testing.T) {
		max := uint32(10)
		importTableType := Table{Max: &max}
		s := newStore()
		s.nameToModule[moduleName] = &ModuleInstance{
			Tables:     []*TableInstance{{Min: importTableType.Min - 1}},
			Exports:    map[string]*Export{name: {Type: ExternTypeTable}},
			ModuleName: moduleName,
		}
		m := &ModuleInstance{Tables: make([]*TableInstance, 1), s: s}
		err := m.resolveImports(context.Background(), &Module{
			ImportPerModule: map[string][]*Import{
				moduleName: {{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: importTableType}},
			},
		})
		require.EqualError(t, err, "import table[test.target]: maximum size mismatch: 10, but actual has no max")
	})
	t.Run("type mismatch", func(t *testing.T) {
		s := newStore()
		s.nameToModule[moduleName] = &ModuleInstance{
			Tables:     []*TableInstance{{Type: RefTypeFuncref}},
			Exports:    map[string]*Export{name: {Type: ExternTypeTable}},
			ModuleName: moduleName,
		}
		m := &ModuleInstance{Tables: make([]*TableInstance, 1), s: s}
		err := m.resolveImports(context.Background(), &Module{
			ImportPerModule: map[string][]*Import{
				moduleName: {{Module: moduleName, Name: name, Type: ExternTypeTable, DescTable: Table{Type: RefTypeExternref}}},
			},
		})
		require.EqualError(t, err, "import table[test.target]: table type mismatch: externref != funcref")
	})
}

var codeEnd = Code{Body: []byte{OpcodeEnd}}

func TestModule_validateTable(t *testing.T) {
	const maxTableIndex = 5
	three := uint32(3)
	tests := []struct {
		name  string
		input *Module
	}{
		{
			name:  "empty",
			input: &Module{},
		},
		{
			name:  "min zero",
			input: &Module{TableSection: []Table{{}}},
		},
		{
			name:  "maximum number of tables",
			input: &Module{TableSection: []Table{{}, {}, {}, {}, {}}},
		},
		{
			name:  "min/max",
			input: &Module{TableSection: []Table{{Min: 1, Max: &three}}},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant derived element offset=0 and one index",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant derived element offset - ignores min on imported table",
			input: &Module{
				ImportTableCount: 1,
				TypeSection:      []FunctionType{{}},
				ImportSection:    []Import{{Type: ExternTypeTable, DescTable: Table{Type: RefTypeFuncref}}},
				FunctionSection:  []Index{0},
				CodeSection:      []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant derived element offset=0 and one index - imported table",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				ImportSection:   []Import{{Type: ExternTypeTable, DescTable: Table{Min: 1, Type: RefTypeFuncref}}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const0},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant derived element offset and two indices",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 3, Type: RefTypeFuncref}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
						Init:       []Index{0, 2},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "imported global derived element offset and one index",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "imported global derived element offset and one index - imported table",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeTable, DescTable: Table{Min: 1, Type: RefTypeFuncref}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "imported global derived element offset - ignores min on imported table",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeTable, DescTable: Table{Type: RefTypeFuncref}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "imported global derived element offset - two indices",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 3, Type: RefTypeFuncref}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{0, 2},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant offset - two inits from globals - funcref",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeFuncref}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeFuncref}},
				},
				ImportGlobalCount: 2,
				TableSection:      []Table{{Min: 10, Type: RefTypeFuncref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x5}},
						Init:       []Index{WrapGlobalIndexAsElementInit(0), WrapGlobalIndexAsElementInit(1)},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
		{
			name: "constant offset - two inits from globals - externref",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeExternref}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeExternref}},
				},
				ImportGlobalCount: 2,
				TableSection:      []Table{{Min: 10, Type: RefTypeExternref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x5}},
						Init:       []Index{elementInitImportedGlobalReferenceType, elementInitImportedGlobalReferenceType | 1},
						Type:       RefTypeExternref,
					},
				},
			},
		},
		{
			name: "mixed elementSegments - const before imported global",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 3, Type: RefTypeFuncref}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
						Init:       []Index{0, 2},
						Type:       RefTypeFuncref,
					},
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{1, 2},
						Type:       RefTypeFuncref,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, _, tables, err := tc.input.AllDeclarations()
			require.NoError(t, err)

			err = tc.input.validateTable(api.CoreFeaturesV1, tables, maxTableIndex)
			require.NoError(t, err)

			err = tc.input.validateTable(api.CoreFeaturesV1, tables, maxTableIndex)
			require.NoError(t, err)
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
				TableSection: []Table{{}, {}, {}, {}, {}, {}},
			},
			expectedErr: "too many tables in a module: 6 given with limit 5",
		},
		{
			name: "type mismatch: unknown ref type",
			input: &Module{
				TableSection: []Table{{Type: RefTypeFuncref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Type: 0xff,
					},
				},
			},
			expectedErr: "element type mismatch: table has funcref but element has unknown(0xff)",
		},
		{
			name: "type mismatch: funcref elem on extern table",
			input: &Module{
				TableSection: []Table{{Type: RefTypeExternref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element type mismatch: table has externref but element has funcref",
		},
		{
			name: "type mismatch: extern elem on funcref table",
			input: &Module{
				TableSection: []Table{{Type: RefTypeFuncref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Type: RefTypeExternref,
					},
				},
			},
			expectedErr: "element type mismatch: table has funcref but element has externref",
		},
		{
			name: "non-nil non-global externref",
			input: &Module{
				TableSection: []Table{{Type: RefTypeFuncref}},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Type: RefTypeExternref,
						Init: []Index{0},
					},
				},
			},
			expectedErr: "element[0].init[0] must be ref.null but was 0",
		},
		{
			name: "constant derived element offset - decode error",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeI32Const,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] couldn't read i32.const parameter: overflows a 32-bit integer",
		},
		{
			name: "constant derived element offset - wrong ValType",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI64Const, Data: const0}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] has an invalid const expression: i64.const",
		},
		{
			name: "constant derived element offset - missing table",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const0}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "unknown table 0 as active element target",
		},
		{
			name: "constant derived element offset exceeds table min",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset puts init beyond table min",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 2, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []Index{0, 0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[1].init exceeds min table size",
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset beyond table min - no init elements",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1, Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)},
						Type:       RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init exceeds min table size",
		},
		{
			name: "constant derived element offset - func index out of range",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []Index{0, 1},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init[1] func index 1 out of range",
		},
		{
			name: "constant derived element offset - global out of range",
			input: &Module{
				ImportGlobalCount: 50,
				TypeSection:       []FunctionType{{}},
				TableSection:      []Table{{Min: 1}},
				FunctionSection:   []Index{0},
				CodeSection:       []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: const1}, Init: []Index{
							elementInitImportedGlobalReferenceType | 1,
							elementInitImportedGlobalReferenceType | 100,
						},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init[1] global index 100 out of range",
		},
		{
			name: "imported global derived element offset - missing table",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "unknown table 0 as active element target",
		},
		{
			name: "imported global derived element offset - func index out of range",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0, 1},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0].init[1] func index 1 out of range",
		},
		{
			name: "imported global derived element offset - wrong ValType",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI64}},
				},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] (global.get 0): import[0].global.ValType != i32",
		},
		{
			name: "imported global derived element offset - decode error",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{
							Opcode: OpcodeGlobalGet,
							Data:   leb128.EncodeUint64(math.MaxUint64),
						},
						Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] couldn't read global.get parameter: overflows a 32-bit integer",
		},
		{
			name: "imported global derived element offset - no imports",
			input: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				GlobalSection:   []Global{{Type: GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0},
						Type: RefTypeFuncref,
					},
				},
			},
			expectedErr: "element[0] (global.get 0): out of range of imported globals",
		},
		{
			name: "imported global derived element offset - no imports are globals",
			input: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeFunc, DescFunc: 0},
				},
				TableSection:    []Table{{Type: RefTypeFuncref}},
				FunctionSection: []Index{0},
				GlobalSection:   []Global{{Type: GlobalType{ValType: ValueTypeI32}}}, // ignored as not imported
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}}, Init: []Index{0},
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
			err = tc.input.validateTable(api.CoreFeaturesV1, tables, maxTableIndex)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

var (
	const0 = leb128.EncodeInt32(0)
	const1 = leb128.EncodeInt32(1)
)

func TestModule_buildTables(t *testing.T) {
	three := uint32(3)
	tests := []struct {
		name            string
		module          *Module
		importedTables  []*TableInstance
		importedGlobals []*GlobalInstance
		expectedTables  []*TableInstance
	}{
		{
			name: "empty",
			module: &Module{
				ElementSection: []ElementSegment{},
			},
		},
		{
			name: "min zero",
			module: &Module{
				TableSection:   []Table{{Type: RefTypeFuncref}},
				ElementSection: []ElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 0), Min: 0, Type: RefTypeFuncref}},
		},
		{
			name: "min/max",
			module: &Module{
				TableSection:   []Table{{Min: 1, Max: &three}},
				ElementSection: []ElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1, Max: &three}},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "constant derived element offset=0 and no index",
			module: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection:  []ElementSegment{},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1}},
		},
		{
			name: "null extern refs",
			module: &Module{
				TableSection: []Table{{Min: 10, Type: RefTypeExternref}},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{5}}, Init: []Index{ElementInitNullReference, ElementInitNullReference, ElementInitNullReference}}, // three null refs.
				},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 10), Min: 10, Type: RefTypeExternref}},
		},
		{
			name: "constant derived element offset=0 and one index",
			module: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 1), Min: 1}},
		},
		{
			name: "constant derived element offset - imported table",
			module: &Module{
				TypeSection:     []FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			importedTables: []*TableInstance{{Min: 2}},
			expectedTables: []*TableInstance{{Min: 2}},
		},
		{
			name: "constant derived element offset=0 and one index - imported table",
			module: &Module{
				TypeSection:     []FunctionType{{}},
				ImportSection:   []Import{{Type: ExternTypeTable, DescTable: Table{Min: 1}}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			importedTables: []*TableInstance{{Min: 1}},
			expectedTables: []*TableInstance{{Min: 1}},
		},
		{
			name: "constant derived element offset and two indices",
			module: &Module{
				TypeSection:     []FunctionType{{}},
				TableSection:    []Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{1}}, Init: []Index{0, 2}},
				},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 3), Min: 3}},
		},
		{ // See: https://github.com/WebAssembly/spec/issues/1427
			name: "imported global derived element offset and no index",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 1}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection:  []ElementSegment{},
			},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTables:  []*TableInstance{{References: make([]Reference, 1), Min: 1}},
		},
		{
			name: "imported global derived element offset and one index",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 1}},
			expectedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
		},
		{
			name: "imported global derived element offset and one index - imported table",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeTable, DescTable: Table{Min: 1}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedTables:  []*TableInstance{{Min: 2, References: []Reference{0, 0}}},
		},
		{
			name: "imported global derived element offset - ignores min on imported table",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeTable, DescTable: Table{}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0}}, Init: []Index{0}},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 1}},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedTables:  []*TableInstance{{Min: 2, References: []Reference{0, 0}}},
		},
		{
			name: "imported global derived element offset - two indices",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 3}, {Min: 100}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{ElementInitNullReference, 2},
						TableIndex: 1,
					},
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x1}},
						Init:       []Index{0, 2},
						TableIndex: 0,
					},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTables: []*TableInstance{
				{References: make([]Reference, 3), Min: 3},
				{References: make([]Reference, 100), Min: 100},
			},
		},
		{
			name: "mixed elementSegments - const before imported global",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI64}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 3}},
				FunctionSection: []Index{0, 0, 0, 0},
				CodeSection:     []Code{codeEnd, codeEnd, codeEnd, codeEnd},
				ElementSection: []ElementSegment{
					{OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{1}}, Init: []Index{0, 2}},
					{OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{1}}, Init: []Index{1, 2}},
				},
			},
			importedGlobals: []*GlobalInstance{
				{Type: GlobalType{ValType: ValueTypeI64}, Val: 3},
				{Type: GlobalType{ValType: ValueTypeI32}, Val: 1},
			},
			expectedTables: []*TableInstance{{References: make([]Reference, 3), Min: 3}},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := &ModuleInstance{
				Tables:  append(tc.importedTables, make([]*TableInstance, len(tc.module.TableSection))...),
				Globals: tc.importedGlobals,
			}
			err := m.buildTables(tc.module, false)
			require.NoError(t, err)

			require.Equal(t, tc.expectedTables, m.Tables)
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
				TypeSection:     []FunctionType{{}},
				ImportSection:   []Import{{Type: ExternTypeTable, DescTable: Table{}}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{2}},
						Init:       []Index{0},
					},
				},
			},
			importedTables: []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			expectedErr:    "element[0].init exceeds min table size",
		},
		{
			name: "imported global derived element offset exceeds table min",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 2}},
			expectedErr:     "element[0].init exceeds min table size",
		},
		{
			name: "imported global derived element offset exceeds table min imported table",
			module: &Module{
				TypeSection: []FunctionType{{}},
				ImportSection: []Import{
					{Type: ExternTypeTable, DescTable: Table{}},
					{Type: ExternTypeGlobal, DescGlobal: GlobalType{ValType: ValueTypeI32}},
				},
				TableSection:    []Table{{Min: 2}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{codeEnd},
				ElementSection: []ElementSegment{
					{
						OffsetExpr: ConstantExpression{Opcode: OpcodeGlobalGet, Data: []byte{0x0}},
						Init:       []Index{0},
					},
				},
			},
			importedTables:  []*TableInstance{{References: make([]Reference, 2), Min: 2}},
			importedGlobals: []*GlobalInstance{{Type: GlobalType{ValType: ValueTypeI32}, Val: 2}},
			expectedErr:     "element[0].init exceeds min table size",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := &ModuleInstance{
				Tables:  append(tc.importedTables, make([]*TableInstance, len(tc.module.TableSection))...),
				Globals: tc.importedGlobals,
			}
			err := m.buildTables(tc.module, false)
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
			actual := table.Grow(tc.delta, 0)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func Test_unwrapElementInitGlobalReference(t *testing.T) {
	actual, ok := unwrapElementInitGlobalReference(12345 | elementInitImportedGlobalReferenceType)
	require.True(t, ok)
	require.Equal(t, actual, uint32(12345))

	actual, ok = unwrapElementInitGlobalReference(12345)
	require.False(t, ok)
	require.Equal(t, actual, uint32(12345))
}

// Test_ElementInitSpecials ensures these special consts are larger than MaximumFunctionIndex so that
// they won't collide with the actual index.
func Test_ElementInitSpecials(t *testing.T) {
	require.True(t, ElementInitNullReference > MaximumFunctionIndex)
	require.True(t, elementInitImportedGlobalReferenceType > MaximumFunctionIndex)
}
