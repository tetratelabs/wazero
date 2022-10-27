package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
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
	functionArgsSizesGet := "args_sizes_get"
	functionFdWrite := "fd_write"
	functionSwap := "swap"

	tests := []struct {
		name, moduleName string
		nameToGoFunc     map[string]interface{}
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
			moduleName: "wasi_snapshot_preview1",
			nameToGoFunc: map[string]interface{}{
				functionArgsSizesGet: argsSizesGet,
				functionFdWrite:      fdWrite,
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
					ModuleName: "wasi_snapshot_preview1",
					FunctionNames: NameMap{
						{Index: 0, Name: "args_sizes_get"},
						{Index: 1, Name: "fd_write"},
					},
				},
			},
		},
		{
			name:       "multi-value",
			moduleName: "swapper",
			nameToGoFunc: map[string]interface{}{
				functionSwap: swap,
			},
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
			m, e := NewHostModule(tc.moduleName, tc.nameToGoFunc, nil,
				api.CoreFeaturesV1|api.CoreFeatureMultiValue)
			require.NoError(t, e)
			requireHostModuleEquals(t, tc.expected, m)
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
		expectedErr      string
	}{
		{
			name:         "not a function",
			nameToGoFunc: map[string]interface{}{"fn": t},
			expectedErr:  "func[.fn] kind != func: ptr",
		},
		{
			name:         "function has multiple results",
			nameToGoFunc: map[string]interface{}{"fn": func(context.Context) (uint32, uint32) { return 0, 0 }},
			expectedErr:  "func[.fn] multiple result types invalid as feature \"multi-value\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := NewHostModule(tc.moduleName, tc.nameToGoFunc, nil, api.CoreFeaturesV1)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
