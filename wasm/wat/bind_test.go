package wat

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestBindIndices(t *testing.T) {
	i32 := wasm.ValueTypeI32
	paramI32I32ResultI32 := &typeFunc{params: []wasm.ValueType{i32, i32}, result: i32}
	paramI32I32I32I32ResultI32 := &typeFunc{params: []wasm.ValueType{i32, i32, i32, i32}, result: i32}
	indexZero, indexOne := &index{numeric: 0}, &index{numeric: 1}

	tests := []struct {
		name            string
		input, expected *module
	}{
		{
			name: "import function: inlined type to numeric index",
			input: &module{
				types: []*typeFunc{paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeInlined: &inlinedTypeFunc{paramI32I32I32I32ResultI32, 0, 0}},
				},
			},
			expected: &module{
				types: []*typeFunc{paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: indexZero},
				},
			},
		},
		{
			name: "import function: multiple inlined types to numeric indices",
			input: &module{
				types: []*typeFunc{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeInlined: &inlinedTypeFunc{paramI32I32I32I32ResultI32, 0, 0}},
				},
			},
			expected: &module{
				types: []*typeFunc{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: indexZero},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: indexOne},
				},
			},
		},
		{
			name: "import function: multiple inlined types to same numeric index",
			input: &module{
				types: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "runtime.args_get",
						typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeInlined: &inlinedTypeFunc{paramI32I32ResultI32, 0, 0}},
				},
			},
			expected: &module{
				types: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "runtime.args_get",
						typeIndex: indexOne},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: indexOne},
				},
			},
		},
		{
			name: "import function: multiple type names to numeric indices",
			input: &module{
				types: []*typeFunc{
					typeFuncEmpty,
					{name: "i32i32_i32", params: []wasm.ValueType{i32, i32}, result: i32},
					{name: "i32i32i32i32_i32", params: []wasm.ValueType{i32, i32, i32, i32}, result: i32},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: &index{ID: "i32i32_i32", line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: &index{ID: "i32i32i32i32_i32", line: 6, col: 76}},
				},
			},
			expected: &module{
				types: []*typeFunc{
					typeFuncEmpty,
					{name: "i32i32_i32", params: []wasm.ValueType{i32, i32}, result: i32},
					{name: "i32i32i32i32_i32", params: []wasm.ValueType{i32, i32, i32, i32}, result: i32},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
			},
		},
		{
			name: "import function: multiple type numeric indices left alone",
			input: &module{
				types: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
			},
			expected: &module{
				types: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
			},
		},
		{
			name: "start: imported function name to numeric index",
			input: &module{
				types: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{funcName: "one", typeIndex: indexZero},
					{funcName: "two", typeIndex: indexZero},
				},
				startFunction: &index{ID: "two", line: 3, col: 9},
			},
			expected: &module{
				types: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{funcName: "one", typeIndex: indexZero},
					{funcName: "two", typeIndex: indexZero},
				},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
		},
		{
			name: "start: imported function numeric index left alone",
			input: &module{
				types:         []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeIndex: indexZero}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
			expected: &module{
				types:         []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeIndex: indexZero}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, tc.input)
		})
	}
}

func TestBindIndices_Errors(t *testing.T) {
	indexZero := &index{}

	tests := []struct {
		name        string
		input       *module
		expectedErr string
	}{
		{
			name: "function type points out of range",
			input: &module{
				types:       []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{{name: "hello", typeIndex: &index{numeric: 1, line: 3, col: 9}}},
			},
			expectedErr: "3:9: index 1 is out of range [0..0] in module.import[0].func.type",
		},
		{
			name: "function type points nowhere",
			input: &module{
				types:       []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{{name: "hello", typeIndex: &index{ID: "main", line: 3, col: 9}}},
			},
			expectedErr: "3:9: unknown ID $main in module.import[0].func.type",
		},
		{
			name: "start points out of range",
			input: &module{
				types:         []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeIndex: indexZero}},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
			expectedErr: "3:9: index 1 is out of range [0..0] in module.start",
		},
		{
			name: "start points nowhere",
			input: &module{
				startFunction: &index{ID: "main", line: 1, col: 16},
			},
			expectedErr: "1:16: unknown ID $main in module.start",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.input)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
