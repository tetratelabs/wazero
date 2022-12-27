package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

func argsSizesGet(ctx context.Context, mod api.Module, resultArgc, resultArgvBufSize uint32) uint32 {
	return 0
}

func fdWrite(ctx context.Context, mod api.Module, fd, iovs, iovsCount, resultSize uint32) uint32 {
	return 0
}

func swap(ctx context.Context, x, y uint32) (uint32, uint32) {
	return y, x
}

func TestNewHostModule(t *testing.T) {
	argsSizesGetName := "args_sizes_get"
	fdWriteName := "fd_write"
	swapName := "swap"

	tests := []struct {
		name, moduleName string
		nameToGoFunc     map[string]interface{}
		funcToNames      map[string]*HostFuncNames
		expected         *Module
	}{
		{
			name:     "empty",
			expected: &Module{},
		},
		{
			name:       "only name",
			moduleName: "test",
			expected:   &Module{NameSection: &NameSection{ModuleName: "test"}},
		},
		{
			name:       "funcs",
			moduleName: wasi_snapshot_preview1.ModuleName,
			nameToGoFunc: map[string]interface{}{
				argsSizesGetName: argsSizesGet,
				fdWriteName:      fdWrite,
			},
			funcToNames: map[string]*HostFuncNames{
				argsSizesGetName: {
					Name:        argsSizesGetName,
					ParamNames:  []string{"result.argc", "result.argv_len"},
					ResultNames: []string{"errno"},
				},
				fdWriteName: {
					Name:        fdWriteName,
					ParamNames:  []string{"fd", "iovs", "iovs_len", "result.size"},
					ResultNames: []string{"errno"},
				},
			},
			expected: &Module{
				TypeSection: []*FunctionType{
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{i32, i32, i32, i32}, Results: []ValueType{i32}},
				},
				FunctionSection: []Index{0, 1},
				CodeSection:     []*Code{MustParseGoReflectFuncCode(argsSizesGet), MustParseGoReflectFuncCode(fdWrite)},
				ExportSection: []*Export{
					{Name: "args_sizes_get", Type: ExternTypeFunc, Index: 0},
					{Name: "fd_write", Type: ExternTypeFunc, Index: 1},
				},
				NameSection: &NameSection{
					ModuleName: wasi_snapshot_preview1.ModuleName,
					FunctionNames: NameMap{
						{Index: 0, Name: "args_sizes_get"},
						{Index: 1, Name: "fd_write"},
					},
					LocalNames: IndirectNameMap{
						{Index: 0, NameMap: NameMap{
							{Index: 0, Name: "result.argc"},
							{Index: 1, Name: "result.argv_len"},
						}},
						{Index: 1, NameMap: NameMap{
							{Index: 0, Name: "fd"},
							{Index: 1, Name: "iovs"},
							{Index: 2, Name: "iovs_len"},
							{Index: 3, Name: "result.size"},
						}},
					},
					ResultNames: IndirectNameMap{
						{Index: 0, NameMap: NameMap{{Index: 0, Name: "errno"}}},
						{Index: 1, NameMap: NameMap{{Index: 0, Name: "errno"}}},
					},
				},
			},
		},
		{
			name:       "multi-value",
			moduleName: "swapper",
			nameToGoFunc: map[string]interface{}{
				swapName: swap,
			},
			funcToNames: map[string]*HostFuncNames{swapName: {}},
			expected: &Module{
				TypeSection:     []*FunctionType{{Params: []ValueType{i32, i32}, Results: []ValueType{i32, i32}}},
				FunctionSection: []Index{0},
				CodeSection:     []*Code{MustParseGoReflectFuncCode(swap)},
				ExportSection:   []*Export{{Name: "swap", Type: ExternTypeFunc, Index: 0}},
				NameSection:     &NameSection{ModuleName: "swapper", FunctionNames: NameMap{{Index: 0, Name: "swap"}}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := NewHostModule(tc.moduleName, tc.nameToGoFunc, tc.funcToNames, api.CoreFeaturesV2)
			require.NoError(t, e)
			requireHostModuleEquals(t, tc.expected, m)
			require.True(t, m.IsHostModule)
		})
	}
}

func requireHostModuleEquals(t *testing.T, expected, actual *Module) {
	// `require.Equal(t, expected, actual)` fails reflect pointers don't match, so brute compare:
	require.Equal(t, expected.TypeSection, actual.TypeSection)
	require.Equal(t, expected.ImportSection, actual.ImportSection)
	require.Equal(t, expected.FunctionSection, actual.FunctionSection)
	require.Equal(t, expected.TableSection, actual.TableSection)
	require.Equal(t, expected.MemorySection, actual.MemorySection)
	require.Equal(t, expected.GlobalSection, actual.GlobalSection)
	require.Equal(t, expected.ExportSection, actual.ExportSection)
	require.Equal(t, expected.StartSection, actual.StartSection)
	require.Equal(t, expected.ElementSection, actual.ElementSection)
	require.Equal(t, expected.DataSection, actual.DataSection)
	require.Equal(t, expected.NameSection, actual.NameSection)

	// Special case because reflect.Value can't be compared with Equals
	// TODO: This is copy/paste with /builder_test.go
	require.Equal(t, len(expected.CodeSection), len(actual.CodeSection))
	for i, c := range expected.CodeSection {
		actualCode := actual.CodeSection[i]
		require.True(t, actualCode.IsHostFunction)
		require.Equal(t, c.GoFunc, actualCode.GoFunc)

		// Not wasm
		require.Nil(t, actualCode.Body)
		require.Nil(t, actualCode.LocalTypes)
	}
}

func TestNewHostModule_Errors(t *testing.T) {
	tests := []struct {
		name, moduleName string
		nameToGoFunc     map[string]interface{}
		funcToNames      map[string]*HostFuncNames
		expectedErr      string
	}{
		{
			name:         "not a function",
			nameToGoFunc: map[string]interface{}{"fn": t},
			funcToNames:  map[string]*HostFuncNames{"fn": {}},
			expectedErr:  "func[.fn] kind != func: ptr",
		},
		{
			name:         "function has multiple results",
			nameToGoFunc: map[string]interface{}{"fn": func() (uint32, uint32) { return 0, 0 }},
			funcToNames:  map[string]*HostFuncNames{"fn": {}},
			expectedErr:  "func[.fn] multiple result types invalid as feature \"multi-value\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := NewHostModule(tc.moduleName, tc.nameToGoFunc, tc.funcToNames, api.CoreFeaturesV1)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
