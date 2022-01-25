package text

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestBindIndices(t *testing.T) {
	i32 := wasm.ValueTypeI32
	paramI32I32ResultI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	paramI32I32I32I32ResultI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}
	indexZero, indexOne := &index{numeric: 0}, &index{numeric: 1}

	tests := []struct {
		name                 string
		inputModule          *module
		inputTypeNameToIndex map[string]wasm.Index
		inputFuncNameToIndex map[string]wasm.Index
		expected             *module
	}{
		{
			name: "import function: inlined type to numeric index",
			inputModule: &module{
				types: []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeInlined: &inlinedTypeFunc{paramI32I32I32I32ResultI32, 0, 0}},
				},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"runtime.fd_write": wasm.Index(0)},
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
			},
		},
		{
			name: "import function: multiple inlined types to numeric indices",
			inputModule: &module{
				types: []*wasm.FunctionType{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
					{typeInlined: &inlinedTypeFunc{paramI32I32I32I32ResultI32, 0, 0}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{
				"runtime.args_sizes_get": wasm.Index(0),
				"runtime.fd_write":       wasm.Index(1),
			},
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
		},
		{
			name: "import function: multiple inlined types to same numeric index",
			inputModule: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{
					{typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
					{typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_get"},
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{
				"runtime.args_sizes_get": wasm.Index(0),
				"runtime.fd_write":       wasm.Index(1),
			},
			expected: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexOne}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_get"},
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
				},
			},
		},
		{
			name: "import function: multiple type names to numeric indices",
			inputModule: &module{
				types: []*wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				typeUses: []*typeUse{
					{typeIndex: &index{ID: "i32i32_i32", line: 5, col: 86}},
					{typeIndex: &index{ID: "i32i32i32i32_i32", line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
			inputTypeNameToIndex: map[string]wasm.Index{
				"i32i32_i32": wasm.Index(1), "i32i32i32i32_i32": wasm.Index(2),
			},
			inputFuncNameToIndex: map[string]wasm.Index{
				"runtime.args_sizes_get": wasm.Index(0),
				"runtime.fd_write":       wasm.Index(1),
			},
			expected: &module{
				types: []*wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
		},
		{
			name: "import function: multiple type numeric indices left alone",
			inputModule: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{
				"runtime.args_sizes_get": wasm.Index(0),
				"runtime.fd_write":       wasm.Index(1),
			},
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
			},
		},
		{
			name: "export imported func",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "bar", exportIndex: wasm.Index(0), funcIndex: &index{ID: "bar", line: 3, col: 22}},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"bar": wasm.Index(0)},
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "bar", exportIndex: wasm.Index(0), funcIndex: &index{numeric: 0, line: 3, col: 22}},
				},
			},
		},
		{
			name: "export imported func twice",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{ID: "bar", line: 3, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{ID: "bar", line: 4, col: 22}},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"bar": wasm.Index(0)},
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{numeric: 0, line: 3, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{numeric: 0, line: 4, col: 22}},
				},
			},
		},
		{
			name: "export different func",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{ID: "bar", line: 4, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{ID: "qux", line: 5, col: 22}},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"bar": wasm.Index(0), "qux": wasm.Index(1)},
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{numeric: 0, line: 4, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{numeric: 1, line: 5, col: 22}},
				},
			},
		},
		{
			name: "start: imported function name to numeric index",
			inputModule: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs:   []*importFunc{{}, {}},
				startFunction: &index{ID: "two", line: 3, col: 9},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"one": wasm.Index(0), "two": wasm.Index(1)},
			expected: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs:   []*importFunc{{}, {}},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
		},
		{
			name: "start: imported function numeric index left alone",
			inputModule: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}},
				importFuncs:   []*importFunc{{name: "hello"}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
			expected: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}},
				importFuncs:   []*importFunc{{name: "hello"}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.inputModule, tc.inputTypeNameToIndex, tc.inputFuncNameToIndex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, tc.inputModule)
		})
	}
}

func TestBindIndices_Errors(t *testing.T) {
	i32 := wasm.ValueTypeI32
	indexZero := &index{}

	tests := []struct {
		name                 string
		inputModule          *module
		inputTypeNameToIndex map[string]wasm.Index
		inputFuncNameToIndex map[string]wasm.Index
		expectedErr          string
	}{
		{
			name: "import function: type points out of range",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: &index{numeric: 1, line: 3, col: 9}}},
				importFuncs: []*importFunc{{name: "hello"}},
			},
			expectedErr: "3:9: index 1 is out of range [0..0] in module.import[0].func.type",
		},
		{
			name: "import function: type points nowhere",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: &index{ID: "main", line: 3, col: 9}}},
				importFuncs: []*importFunc{{name: "hello"}},
			},
			expectedErr: "3:9: unknown ID $main in module.import[0].func.type",
		},
		{
			name: "import function: inlined type doesn't match indexed",
			inputModule: &module{
				types: []*wasm.FunctionType{{}},
				typeUses: []*typeUse{
					{typeIndex: indexZero, typeInlined: &inlinedTypeFunc{&wasm.FunctionType{Params: []wasm.ValueType{i32, i32}}, 3, 9}},
				},
				importFuncs: []*importFunc{{module: "", name: "hello"}},
			},
			expectedErr: "3:9: inlined type doesn't match type index 0 in module.import[0].func.type",
		},
		{
			name: "function: type points out of range",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: &index{numeric: 1, line: 3, col: 9}}},
				code:     []*wasm.Code{{Body: end}},
			},
			expectedErr: "3:9: index 1 is out of range [0..0] in module.func[0].type",
		},
		{
			name: "function: type points nowhere",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: &index{ID: "main", line: 3, col: 9}}},
				code:     []*wasm.Code{{Body: end}},
			},
			expectedErr: "3:9: unknown ID $main in module.func[0].type",
		},
		{
			name: "function: inlined type doesn't match indexed",
			inputModule: &module{
				types: []*wasm.FunctionType{{}},
				typeUses: []*typeUse{
					{typeIndex: indexZero,
						typeInlined: &inlinedTypeFunc{&wasm.FunctionType{Params: []wasm.ValueType{i32, i32}}, 3, 9}},
				},
				code: []*wasm.Code{{Body: end}},
			},
			expectedErr: "3:9: inlined type doesn't match type index 0 in module.func[0].type",
		},
		{
			name: "export func points out of range",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "bar", exportIndex: wasm.Index(0), funcIndex: &index{numeric: 3, line: 3, col: 22}},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"bar": wasm.Index(0)},
			expectedErr:          "3:22: index 3 is out of range [0..0] in module.exports[0].func",
		},
		{
			name: "export func points nowhere",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "bar", exportIndex: wasm.Index(0), funcIndex: &index{ID: "qux", line: 3, col: 22}},
				},
			},
			inputFuncNameToIndex: map[string]wasm.Index{"bar": wasm.Index(0)},
			expectedErr:          "3:22: unknown ID $qux in module.exports[0].func",
		},
		{
			name: "start points out of range",
			inputModule: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}},
				importFuncs:   []*importFunc{{name: "hello"}},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
			expectedErr: "3:9: index 1 is out of range [0..0] in module.start",
		},
		{
			name: "start points nowhere",
			inputModule: &module{
				startFunction: &index{ID: "main", line: 1, col: 16},
			},
			expectedErr: "1:16: unknown ID $main in module.start",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.inputModule, tc.inputTypeNameToIndex, tc.inputFuncNameToIndex)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestMergeLocalNames(t *testing.T) {
	i32 := wasm.ValueTypeI32
	paramI32I32ResultI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	indexZero, indexOne := &index{numeric: 0}, &index{numeric: 1}
	localGet0End := []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}

	tests := []struct {
		name                string
		inputModule         *module
		inputTypeParamNames map[wasm.Index]idContext
		expected            wasm.IndirectNameMap
	}{
		{
			name: "no parameter names",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				names:       &wasm.NameSection{},
			},
		},
		{
			name: "type parameter names, but no import function parameter names",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				names:       &wasm.NameSection{},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"argv": wasm.Index(0), "argv_buf": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "import function parameter names, but no type parameter names",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "type parameter names, but no import function parameter names - function 2",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "", name: ""}, {module: "wasi_snapshot_preview1", name: "args_get"}},
				names:       &wasm.NameSection{},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"argv": wasm.Index(0), "argv_buf": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "import function parameter names, but no type parameter names - function 2",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "", name: ""}, {module: "wasi_snapshot_preview1", name: "args_get"}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "conflict on import function parameter names and type parameter names",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"x": wasm.Index(0), "y": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "type parameter names, but no function parameter names",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names:    &wasm.NameSection{},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"argv": wasm.Index(0), "argv_buf": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "function parameter names, but no type parameter names",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "type parameter names, but no function parameter names - function 2",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: end}, {Body: localGet0End}},
				names:    &wasm.NameSection{},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"argv": wasm.Index(0), "argv_buf": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "function parameter names, but no type parameter names - function 2",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: end}, {Body: localGet0End}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "conflict on function parameter names and type parameter names",
			inputModule: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
					},
				},
			},
			inputTypeParamNames: map[wasm.Index]idContext{
				wasm.Index(1): {"x": wasm.Index(0), "y": wasm.Index(1)},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "argv"}, {Index: wasm.Index(1), Name: "argv_buf"}}},
			},
		},
		{
			name: "import and module defined function have same type, but different parameter names",
			inputModule: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "", name: ""}},
				code:        []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(0), Name: "y"}}},
						{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "l"}, {Index: wasm.Index(0), Name: "r"}}},
					},
				},
			},
			expected: wasm.IndirectNameMap{
				{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(0), Name: "y"}}},
				{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "l"}, {Index: wasm.Index(0), Name: "r"}}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, mergeLocalNames(tc.inputModule, tc.inputTypeParamNames))
		})
	}
}
