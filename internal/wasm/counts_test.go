package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_ImportFuncCount(t *testing.T) {
	tests := []struct {
		name     string
		input    *Module
		expected uint32
	}{
		{
			name:  "none",
			input: &Module{},
		},
		{
			name:  "none with function section",
			input: &Module{FunctionSection: []Index{0}},
		},
		{
			name:     "one",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeFunc}}},
			expected: 1,
		},
		{
			name:     "one with function section",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeFunc}}, FunctionSection: []Index{0}},
			expected: 1,
		},
		{
			name:     "one with other imports",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeFunc}, {Type: ExternTypeMemory}}},
			expected: 1,
		},
		{
			name:     "two",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeFunc}, {Type: ExternTypeFunc}}},
			expected: 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.ImportFuncCount())
		})
	}
}

func TestModule_ImportTableCount(t *testing.T) {
	tests := []struct {
		name     string
		input    *Module
		expected uint32
	}{
		{
			name:  "none",
			input: &Module{},
		},
		{
			name:  "none with table section",
			input: &Module{TableSection: []*Table{{Min: 1, Max: nil}}},
		},
		{
			name:     "one",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeTable}}},
			expected: 1,
		},
		{
			name: "one with table section",
			input: &Module{
				ImportSection: []*Import{{Type: ExternTypeTable}},
				TableSection:  []*Table{{Min: 1, Max: nil}},
			},
			expected: 1,
		},
		{
			name:     "one with other imports",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeTable}, {Type: ExternTypeMemory}}},
			expected: 1,
		},
		{
			name:     "two",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeTable}, {Type: ExternTypeTable}}},
			expected: 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.ImportTableCount())
		})
	}
}

// TODO: once we fix up-front validation, this only needs to check zero or one
func TestModule_ImportMemoryCount(t *testing.T) {
	tests := []struct {
		name     string
		input    *Module
		expected uint32
	}{
		{
			name:  "none",
			input: &Module{},
		},
		{
			name:  "none with memory section",
			input: &Module{MemorySection: &Memory{Min: 1}},
		},
		{
			name:     "one",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeMemory}}},
			expected: 1,
		},
		{
			name: "one with memory section",
			input: &Module{
				ImportSection: []*Import{{Type: ExternTypeMemory}},
				MemorySection: &Memory{Min: 1},
			},
			expected: 1,
		},
		{
			name:     "one with other imports",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeMemory}, {Type: ExternTypeTable}}},
			expected: 1,
		},
		{
			name:     "two",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeMemory}, {Type: ExternTypeMemory}}},
			expected: 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.ImportMemoryCount())
		})
	}
}

func TestModule_ImportGlobalCount(t *testing.T) {
	tests := []struct {
		name     string
		input    *Module
		expected uint32
	}{
		{
			name:  "none",
			input: &Module{},
		},
		{
			name:  "none with global section",
			input: &Module{GlobalSection: []*Global{{Type: &GlobalType{ValType: ValueTypeI64}}}},
		},
		{
			name:     "one",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeGlobal}}},
			expected: 1,
		},
		{
			name: "one with global section",
			input: &Module{
				ImportSection: []*Import{{Type: ExternTypeGlobal}},
				GlobalSection: []*Global{{Type: &GlobalType{ValType: ValueTypeI64}}},
			},
			expected: 1,
		},
		{
			name:     "one with other imports",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeGlobal}, {Type: ExternTypeMemory}}},
			expected: 1,
		},
		{
			name:     "two",
			input:    &Module{ImportSection: []*Import{{Type: ExternTypeGlobal}, {Type: ExternTypeGlobal}}},
			expected: 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.ImportGlobalCount())
		})
	}
}

func TestModule_SectionElementCount(t *testing.T) {
	i32, f32 := ValueTypeI32, ValueTypeF32
	zero := uint32(0)
	empty := &ConstantExpression{Opcode: OpcodeI32Const, Data: const0}

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
			name:     "NameSection",
			input:    &Module{NameSection: &NameSection{ModuleName: "simple"}},
			expected: map[string]uint32{"custom": 1},
		},
		{
			name: "TypeSection",
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
			name: "TypeSection and ImportSection",
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
			name: "TypeSection, FunctionSection, CodeSection, ExportSection and StartSection",
			input: &Module{
				TypeSection:     []*FunctionType{{}},
				FunctionSection: []Index{0},
				CodeSection: []*Code{
					{Body: []byte{OpcodeLocalGet, 0, OpcodeLocalGet, 1, OpcodeI32Add, OpcodeEnd}},
				},
				ExportSection: []*Export{
					{Name: "AddInt", Type: ExternTypeFunc, Index: Index(0)},
				},
				StartSection: &zero,
			},
			expected: map[string]uint32{"code": 1, "export": 1, "function": 1, "start": 1, "type": 1},
		},
		{
			name: "MemorySection and DataSection",
			input: &Module{
				MemorySection: &Memory{Min: 1},
				DataSection:   []*DataSegment{{OffsetExpression: empty}},
			},
			expected: map[string]uint32{"data": 1, "memory": 1},
		},
		{
			name: "TableSection and ElementSection",
			input: &Module{
				TableSection:   []*Table{{Min: 1}},
				ElementSection: []*ElementSegment{{OffsetExpr: empty}},
			},
			expected: map[string]uint32{"element": 1, "table": 1},
		},
		{
			name: "TableSection (multiple tables) and ElementSection",
			input: &Module{
				TableSection:   []*Table{{Min: 1}, {Min: 2}},
				ElementSection: []*ElementSegment{{OffsetExpr: empty}},
			},
			expected: map[string]uint32{"element": 1, "table": 2},
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
