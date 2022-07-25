package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_BuildFunctionDefinitions(t *testing.T) {
	nopCode := &Code{Body: []byte{OpcodeEnd}}
	fn := func() {}
	tests := []struct {
		name            string
		m               *Module
		expected        []*FunctionDefinition
		expectedImports []api.FunctionDefinition
		expectedExports map[string]api.FunctionDefinition
	}{
		{
			name:            "no exports",
			m:               &Module{},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name: "no functions",
			m: &Module{
				ExportSection: []*Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection: []*Global{{}},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name: "host func go",
			m: &Module{
				TypeSection:     []*FunctionType{v_v},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{MustParseGoFuncCode(fn)},
			},
			expected: []*FunctionDefinition{
				{
					index:     0,
					debugName: ".$0",
					goFunc:    MustParseGoFuncCode(fn).GoFunc,
					funcType:  v_v,
				},
			},
			expectedExports: map[string]api.FunctionDefinition{},
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
				CodeSection: []*Code{
					{Body: []byte{OpcodeEnd}},
					{Body: []byte{OpcodeEnd}},
					{Body: []byte{OpcodeEnd}},
				},
				TypeSection: []*FunctionType{
					v_v,
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expected: []*FunctionDefinition{
				{
					index:       0,
					debugName:   ".$0",
					exportNames: []string{"function_index=0"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				{
					index:       1,
					debugName:   ".$1",
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				{
					index:       2,
					debugName:   ".$2",
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
			expectedExports: map[string]api.FunctionDefinition{
				"function_index=0": &FunctionDefinition{
					index:       0,
					debugName:   ".$0",
					exportNames: []string{"function_index=0"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				"function_index=1": &FunctionDefinition{
					index:       1,
					exportNames: []string{"function_index=1"},
					debugName:   ".$1",
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				"function_index=2": &FunctionDefinition{
					index:       2,
					debugName:   ".$2",
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
				CodeSection:     []*Code{{Body: []byte{OpcodeEnd}}, {Body: []byte{OpcodeEnd}}},
				TypeSection: []*FunctionType{
					v_v,
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expected: []*FunctionDefinition{
				{
					index:       0,
					debugName:   ".$0",
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				{
					index:       1,
					debugName:   ".$1",
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				{
					index:       2,
					debugName:   ".$2",
					exportNames: []string{"function_index=2"},
					funcType:    v_v,
				},
			},
			expectedImports: []api.FunctionDefinition{
				&FunctionDefinition{
					index:       0,
					debugName:   ".$0",
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			expectedExports: map[string]api.FunctionDefinition{
				"imported_function": &FunctionDefinition{
					index:       0,
					debugName:   ".$0",
					importDesc:  &[2]string{"", ""},
					exportNames: []string{"imported_function"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
				"function_index=1": &FunctionDefinition{
					index:       1,
					debugName:   ".$1",
					exportNames: []string{"function_index=1"},
					funcType:    &FunctionType{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
				},
				"function_index=2": &FunctionDefinition{
					index:       2,
					debugName:   ".$2",
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
				CodeSection:     []*Code{nopCode, nopCode, nopCode, nopCode, nopCode},
			},
			expected: []*FunctionDefinition{
				{moduleName: "module", index: 0, debugName: "module.$0", importDesc: &[2]string{"i", "f"}, funcType: v_v},
				{moduleName: "module", index: 1, debugName: "module.$1", funcType: v_v},
				{moduleName: "module", index: 2, debugName: "module.two", funcType: v_v, name: "two"},
				{moduleName: "module", index: 3, debugName: "module.$3", funcType: v_v},
				{moduleName: "module", index: 4, debugName: "module.four", funcType: v_v, name: "four"},
				{moduleName: "module", index: 5, debugName: "module.five", funcType: v_v, name: "five"},
			},
			expectedImports: []api.FunctionDefinition{
				&FunctionDefinition{moduleName: "module", index: 0, debugName: "module.$0", importDesc: &[2]string{"i", "f"}, funcType: v_v},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.m.BuildFunctionDefinitions()
			require.Equal(t, tc.expected, tc.m.FunctionDefinitionSection)
			require.Equal(t, tc.expectedImports, tc.m.ImportedFunctions())
			require.Equal(t, tc.expectedExports, tc.m.ExportedFunctions())
		})
	}
}
