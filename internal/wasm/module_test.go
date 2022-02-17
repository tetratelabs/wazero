package internalwasm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestFunctionType_String(t *testing.T) {
	for _, tc := range []struct {
		functype *FunctionType
		exp      string
	}{
		{functype: &FunctionType{}, exp: "null_null"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI32}}, exp: "i32_null"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeF64}}, exp: "i32f64_null"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeF64}}, exp: "f32i32f64_null"},
		{functype: &FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI64}}, exp: "null_i64"},
		{functype: &FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeF32}}, exp: "null_i64f32"},
		{functype: &FunctionType{Results: []wasm.ValueType{wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeF64}}, exp: "null_f32i32f64"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI64}}, exp: "i32_i64"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeF32}, Results: []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeF32}}, exp: "i64f32_i64f32"},
		{functype: &FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64}, Results: []wasm.ValueType{wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeF64}}, exp: "i64f32f64_f32i32f64"},
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
		input    wasm.SectionID
		expected string
	}{
		{"custom", wasm.SectionIDCustom, "custom"},
		{"type", wasm.SectionIDType, "type"},
		{"import", wasm.SectionIDImport, "import"},
		{"function", wasm.SectionIDFunction, "function"},
		{"table", wasm.SectionIDTable, "table"},
		{"memory", wasm.SectionIDMemory, "memory"},
		{"global", wasm.SectionIDGlobal, "global"},
		{"export", wasm.SectionIDExport, "export"},
		{"start", wasm.SectionIDStart, "start"},
		{"element", wasm.SectionIDElement, "element"},
		{"code", wasm.SectionIDCode, "code"},
		{"data", wasm.SectionIDData, "data"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, wasm.SectionIDName(tc.input))
		})
	}
}

func TestExportKindName(t *testing.T) {
	tests := []struct {
		name     string
		input    wasm.ExportKind
		expected string
	}{
		{"func", wasm.ExportKindFunc, "func"},
		{"table", wasm.ExportKindTable, "table"},
		{"mem", wasm.ExportKindMemory, "mem"},
		{"global", wasm.ExportKindGlobal, "global"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, wasm.ExportKindName(tc.input))
		})
	}
}

func TestValueTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    wasm.ValueType
		expected string
	}{
		{"i32", wasm.ValueTypeI32, "i32"},
		{"i64", wasm.ValueTypeI64, "i64"},
		{"f32", wasm.ValueTypeF32, "f32"},
		{"f64", wasm.ValueTypeF64, "f64"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, wasm.ValueTypeName(tc.input))
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
				ImportSection:   []*Import{{Kind: wasm.ImportKindFunc, DescFunc: 10000}},
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
				ImportSection: []*Import{{Kind: wasm.ImportKindFunc, DescFunc: 10000}},
			},
			expectedFunctions: []Index{10000},
		},
		// Globals.
		{
			module: &Module{
				ImportSection: []*Import{{Kind: wasm.ImportKindGlobal, DescGlobal: &GlobalType{Mutable: false}}},
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
				ImportSection: []*Import{{Kind: wasm.ImportKindGlobal, DescGlobal: &GlobalType{Mutable: false}}},
			},
			expectedGlobals: []*GlobalType{{Mutable: false}},
		},
		// Memories.
		{
			module: &Module{
				ImportSection: []*Import{{Kind: wasm.ImportKindMemory, DescMem: &LimitsType{Min: 1}}},
				MemorySection: []*MemoryType{{Min: 100}},
			},
			expectedMemories: []*MemoryType{{Min: 1}, {Min: 100}},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Kind: wasm.ImportKindMemory, DescMem: &LimitsType{Min: 1}}},
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
				ImportSection: []*Import{{Kind: wasm.ImportKindTable, DescTable: &TableType{Limit: &LimitsType{Min: 1}}}},
				TableSection:  []*TableType{{Limit: &LimitsType{Min: 10}}},
			},
			expectedTables: []*TableType{{Limit: &LimitsType{Min: 1}}, {Limit: &LimitsType{Min: 10}}},
		},
		{
			module: &Module{
				ImportSection: []*Import{{Kind: wasm.ImportKindTable, DescTable: &TableType{Limit: &LimitsType{Min: 1}}}},
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
	i32, f32 := wasm.ValueTypeI32, wasm.ValueTypeF32
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
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
			},
			expected: map[string]uint32{"type": 3},
		},
		{
			name: "type and import section",
			input: &Module{
				TypeSection: []*FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				ImportSection: []*Import{
					{
						Module: "Math", Name: "Mul",
						Kind:     wasm.ImportKindFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Kind:     wasm.ImportKindFunc,
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
					"AddInt": {Name: "AddInt", Kind: wasm.ExportKindFunc, Index: Index(0)},
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
			for i := wasm.SectionID(0); i <= wasm.SectionIDData; i++ {
				if size := tc.input.SectionElementCount(i); size > 0 {
					actual[wasm.SectionIDName(i)] = size
				}
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}
