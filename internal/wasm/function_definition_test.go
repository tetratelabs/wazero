package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_BuildFunctionDefinitions(t *testing.T) {
	nopCode := &Code{nil, []byte{OpcodeEnd}}
	tests := []struct {
		name                             string
		m                                *Module
		expected                         []*functionDefinition
		expectedImports, expectedExports []api.FunctionDefinition
	}{
		{
			name: "no exports",
			m:    &Module{},
		},
		{
			name: "no functions",
			m: &Module{
				ExportSection: []*Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection: []*Global{{}},
			},
		},
		{
			name: "without imports",
			m: &Module{
				ExportSection: []*Export{
					{Name: "function_index=0", Type: ExternTypeFunc, Index: 0},
					{Name: "function_index=2", Type: ExternTypeFunc, Index: 2},
					{Name: "", Type: ExternTypeGlobal, Index: 0},
					{Name: "function_index=1", Type: ExternTypeFunc, Index: 1},
				},
				GlobalSection:   []*Global{{}},
				FunctionSection: []Index{1, 2, 0},
				TypeSection: []*FunctionType{
					v_v,
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expected: []*functionDefinition{
				{
					index:       0,
					exportNames: []string{"function_index=0"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				{
					index:       1,
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				{
					index:       2,
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
			expectedExports: []api.FunctionDefinition{
				&functionDefinition{
					index:       0,
					exportNames: []string{"function_index=0"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				&functionDefinition{
					index:       1,
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				&functionDefinition{
					index:       2,
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
		},
		{
			name: "with imports",
			m: &Module{
				ImportSection: []*Import{{
					Type:     ExternTypeFunc,
					DescFunc: 2, // Index of type.
				}},
				ExportSection: []*Export{
					{Name: "imported_function", Type: ExternTypeFunc, Index: 0},
					{Name: "function_index=1", Type: ExternTypeFunc, Index: 1},
					{Name: "function_index=2", Type: ExternTypeFunc, Index: 2},
				},
				FunctionSection: []Index{1, 0},
				TypeSection: []*FunctionType{
					v_v,
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expected: []*functionDefinition{
				{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				{
					index:       1,
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				{
					index:       2,
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
			expectedImports: []api.FunctionDefinition{
				&functionDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expectedExports: []api.FunctionDefinition{
				&functionDefinition{
					index:       0,
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				&functionDefinition{
					index:       1,
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				&functionDefinition{
					index:       2,
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
		},
		{
			name: "with names",
			m: &Module{
				TypeSection:   []*FunctionType{v_v},
				ImportSection: []*Import{{Module: "i", Name: "f", Type: ExternTypeFunc}},
				NameSection: &NameSection{
					ModuleName: "module",
					FunctionNames: NameMap{
						{Index: Index(2), Name: "two"},
						{Index: Index(4), Name: "four"},
						{Index: Index(5), Name: "five"},
					},
				},
				FunctionSection: []Index{0, 0, 0, 0, 0},
				CodeSection:     []*Code{nopCode, nopCode, nopCode, nopCode},
			},
			expected: []*functionDefinition{
				{moduleName: "module", index: 0, importDesc: &[2]string{"i", "f"}, funcType: v_v},
				{moduleName: "module", index: 1, funcType: v_v},
				{moduleName: "module", index: 2, funcType: v_v, name: "two"},
				{moduleName: "module", index: 3, funcType: v_v},
				{moduleName: "module", index: 4, funcType: v_v, name: "four"},
				{moduleName: "module", index: 5, funcType: v_v, name: "five"},
			},
			expectedImports: []api.FunctionDefinition{
				&functionDefinition{moduleName: "module", index: 0, importDesc: &[2]string{"i", "f"}, funcType: v_v},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.m.BuildFunctionDefinitions()
			require.Equal(t, tc.expected, tc.m.functionDefinitions)
			require.Equal(t, tc.expectedImports, tc.m.ImportedFunctions())
			require.Equal(t, tc.expectedExports, tc.m.ExportedFunctions())
		})
	}
}
