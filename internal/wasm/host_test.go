package wasm

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasip1"
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
	t.Run("empty name not allowed", func(t *testing.T) {
		_, err := NewHostModule("", nil, nil, api.CoreFeaturesV2)
		require.Error(t, err)
	})

	swapName := "swap"
	tests := []struct {
		name, moduleName string
		exportNames      []string
		nameToHostFunc   map[string]*HostFunc
		expected         *Module
	}{
		{
			name:       "only name",
			moduleName: "test",
			expected:   &Module{NameSection: &NameSection{ModuleName: "test"}},
		},
		{
			name:        "funcs",
			moduleName:  InternalModuleName,
			exportNames: []string{ArgsSizesGetName, FdWriteName},
			nameToHostFunc: map[string]*HostFunc{
				ArgsSizesGetName: {
					ExportName:  ArgsSizesGetName,
					ParamNames:  []string{"result.argc", "result.argv_len"},
					ResultNames: []string{"errno"},
					Code:        Code{GoFunc: argsSizesGet},
				},
				FdWriteName: {
					ExportName:  FdWriteName,
					ParamNames:  []string{"fd", "iovs", "iovs_len", "result.size"},
					ResultNames: []string{"errno"},
					Code:        Code{GoFunc: fdWrite},
				},
			},
			expected: &Module{
				TypeSection: []FunctionType{
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{i32, i32, i32, i32}, Results: []ValueType{i32}},
				},
				FunctionSection: []Index{0, 1},
				CodeSection:     []Code{MustParseGoReflectFuncCode(argsSizesGet), MustParseGoReflectFuncCode(fdWrite)},
				ExportSection: []Export{
					{Name: ArgsSizesGetName, Type: ExternTypeFunc, Index: 0},
					{Name: FdWriteName, Type: ExternTypeFunc, Index: 1},
				},
				Exports: map[string]*Export{
					ArgsSizesGetName: {Name: ArgsSizesGetName, Type: ExternTypeFunc, Index: 0},
					FdWriteName:      {Name: FdWriteName, Type: ExternTypeFunc, Index: 1},
				},
				NameSection: &NameSection{
					ModuleName: InternalModuleName,
					FunctionNames: NameMap{
						{Index: 0, Name: ArgsSizesGetName},
						{Index: 1, Name: FdWriteName},
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
			name:           "multi-value",
			moduleName:     "swapper",
			exportNames:    []string{swapName},
			nameToHostFunc: map[string]*HostFunc{swapName: {ExportName: swapName, Code: Code{GoFunc: swap}}},
			expected: &Module{
				TypeSection:     []FunctionType{{Params: []ValueType{i32, i32}, Results: []ValueType{i32, i32}}},
				FunctionSection: []Index{0},
				CodeSection:     []Code{MustParseGoReflectFuncCode(swap)},
				ExportSection:   []Export{{Name: "swap", Type: ExternTypeFunc, Index: 0}},
				Exports:         map[string]*Export{"swap": {Name: "swap", Type: ExternTypeFunc, Index: 0}},
				NameSection:     &NameSection{ModuleName: "swapper", FunctionNames: NameMap{{Index: 0, Name: "swap"}}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := NewHostModule(tc.moduleName, tc.exportNames, tc.nameToHostFunc, api.CoreFeaturesV2)
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
		require.Equal(t, c.GoFunc, actualCode.GoFunc)

		// Not wasm
		require.Nil(t, actualCode.Body)
		require.Nil(t, actualCode.LocalTypes)
	}
}

func TestNewHostModule_Errors(t *testing.T) {
	tests := []struct {
		name, moduleName string
		exportNames      []string
		nameToHostFunc   map[string]*HostFunc
		expectedErr      string
	}{
		{
			name:           "not a function",
			moduleName:     "modname",
			exportNames:    []string{"fn"},
			nameToHostFunc: map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: t}}},
			expectedErr:    "func[modname.fn] kind != func: ptr",
		},
		{
			name:           "function has multiple results",
			moduleName:     "yetanother",
			exportNames:    []string{"fn"},
			nameToHostFunc: map[string]*HostFunc{"fn": {ExportName: "fn", Code: Code{GoFunc: func() (uint32, uint32) { return 0, 0 }}}},
			expectedErr:    "func[yetanother.fn] multiple result types invalid as feature \"multi-value\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := NewHostModule(tc.moduleName, tc.exportNames, tc.nameToHostFunc, api.CoreFeaturesV1)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
