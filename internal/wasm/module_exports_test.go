package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_ExportedFunctions(t *testing.T) {
	tests := []struct {
		name string
		m    *Module
		exp  []api.ExportedFunction
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
					{Params: []ValueType{}, Results: []ValueType{}},
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			exp: []api.ExportedFunction{
				&exportedFunction{
					exportedName: "function_index=0",
					params:       []ValueType{ValueTypeF64, ValueTypeI32},
					results:      []ValueType{ValueTypeV128, ValueTypeI64},
				},
				&exportedFunction{
					exportedName: "function_index=2", params: []ValueType{}, results: []ValueType{},
				},
				&exportedFunction{
					exportedName: "function_index=1",
					params:       []ValueType{ValueTypeF64, ValueTypeF32},
					results:      []ValueType{ValueTypeI64},
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
					{Params: []ValueType{}, Results: []ValueType{}},
					{Params: []ValueType{ValueTypeF64, ValueTypeI32}, Results: []ValueType{ValueTypeV128, ValueTypeI64}},
					{Params: []ValueType{ValueTypeF64, ValueTypeF32}, Results: []ValueType{ValueTypeI64}},
				},
			},
			exp: []api.ExportedFunction{
				&exportedFunction{
					exportedName: "imported_function",
					params:       []ValueType{ValueTypeF64, ValueTypeF32},
					results:      []ValueType{ValueTypeI64},
				},
				&exportedFunction{
					exportedName: "function_index=1",
					params:       []ValueType{ValueTypeF64, ValueTypeI32},
					results:      []ValueType{ValueTypeV128, ValueTypeI64},
				},
				&exportedFunction{
					exportedName: "function_index=2",
					params:       []ValueType{},
					results:      []ValueType{},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.m.ExportedFunctions()
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestExportedFunction(t *testing.T) {
	f := &exportedFunction{exportedName: "abc", params: []ValueType{ValueTypeI32}, results: []ValueType{ValueTypeV128}}
	require.Equal(t, "abc", f.exportedName)
	require.Equal(t, []ValueType{ValueTypeI32}, f.ParamTypes())
	require.Equal(t, []ValueType{ValueTypeV128}, f.ResultTypes())
}
