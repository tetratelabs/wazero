package wat

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestParseModule(t *testing.T) {
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64
	tests := []struct {
		name     string
		input    string
		expected *module
	}{
		{
			name:     "empty",
			input:    "(module)",
			expected: &module{},
		},
		{
			name:     "only name",
			input:    "(module $tools)",
			expected: &module{name: "$tools"},
		},
		{
			name:  "import func empty",
			input: "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &module{
				types:       []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
			},
		},
		{
			name:  "import func empty twice",
			input: "(module (import \"foo\" \"bar\" (func)) (import \"baz\" \"qux\" (func)))", // ok empty sig
			expected: &module{
				types: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "foo", name: "bar"},
					{importIndex: 1, module: "baz", name: "qux"},
				},
			},
		},
		{
			name: "import func inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &module{
				types: []*typeFunc{{params: []wasm.ValueType{i32, i32, i32, i32}, results: []wasm.ValueType{i32}}},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write"},
				},
			},
		},
		{
			name: "import func inlined type no result",
			input: `(module
	(import "wasi_snapshot_preview1" "proc_exit" (func $runtime.proc_exit (param i32)))
)`,
			expected: &module{
				types: []*typeFunc{{params: []wasm.ValueType{i32}}},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "proc_exit", funcName: "$runtime.proc_exit"},
				},
			},
		},
		{
			name: "import func inlined type different param types",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
)`,
			expected: &module{
				types: []*typeFunc{{
					params:  []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
					results: []wasm.ValueType{i32},
				}},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "path_open", funcName: "$runtime.path_open"},
				},
			},
		},
		{
			name: "multiple import func different inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &module{
				types: []*typeFunc{
					{params: []wasm.ValueType{i32, i32}, results: []wasm.ValueType{i32}},
					{params: []wasm.ValueType{i32, i32, i32, i32}, results: []wasm.ValueType{i32}},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, typeIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get"},
					{importIndex: 1, typeIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write"},
				},
			},
		},
		{
			name: "multiple import func same inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (param i32 i32) (result i32)))
)`,
			expected: &module{
				types: []*typeFunc{
					{params: []wasm.ValueType{i32, i32}, results: []wasm.ValueType{i32}},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, typeIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "$runtime.args_get"},
					{importIndex: 1, typeIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get"},
				},
			},
		},
		{
			name:     "start function", // TODO: this is pointing to a funcidx not in the source!
			input:    "(module (start $main))",
			expected: &module{startFunction: &startFunction{"$main", 1, 16}},
		},
		{
			name: "start imported function by name",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &module{
				types:         []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", funcName: "$hello"}},
				startFunction: &startFunction{"$hello", 3, 9},
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &module{
				types:         []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0}},
				startFunction: &startFunction{"0", 3, 9},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := parseModule([]byte(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, m)
		})
	}
}

func TestParseModule_Errors(t *testing.T) {
	tests := []struct{ name, input, expectedErr string }{
		{
			name:        "no module",
			input:       "()",
			expectedErr: "1:2: expected field, but found )",
		},
		{
			name:        "double module",
			input:       "(module) (module)",
			expectedErr: "1:10: unexpected (",
		},
		{
			name:        "module invalid name",
			input:       "(module test)", // must start with $
			expectedErr: "1:9: unexpected keyword: test in module",
		},
		{
			name:        "module double name",
			input:       "(module $foo $bar)",
			expectedErr: "1:14: redundant name: $bar in module",
		},
		{
			name:        "module empty field",
			input:       "(module $foo ())",
			expectedErr: "1:15: expected field, but found ) in module",
		},
		{
			name:        "module trailing )",
			input:       "(module $foo ))",
			expectedErr: "1:15: found ')' before '('",
		},
		{
			name:        "import missing module",
			input:       "(module (import))",
			expectedErr: "1:16: missing module and name in module.import[0]",
		},
		{
			name:        "import missing name",
			input:       "(module (import \"\"))",
			expectedErr: "1:19: missing name in module.import[0]",
		},
		{
			name:        "import unquoted module",
			input:       "(module (import foo bar))",
			expectedErr: "1:17: unexpected keyword: foo in module.import[0]",
		},
		{
			name:        "import double name",
			input:       "(module (import \"foo\" \"bar\" \"baz\")",
			expectedErr: "1:29: redundant name: baz in module.import[0]",
		},
		{
			name:        "import missing desc",
			input:       "(module (import \"foo\" \"bar\"))",
			expectedErr: "1:28: missing description field in module.import[0]",
		},
		{
			name:        "import empty desc",
			input:       "(module (import \"foo\" \"bar\"())",
			expectedErr: "1:29: expected field, but found ) in module.import[0]",
		},
		{
			name:        "import func invalid name",
			input:       "(module (import \"foo\" \"bar\" (func baz)))",
			expectedErr: "1:35: unexpected keyword: baz in module.import[0].func",
		},
		{
			name:        "import func missing param0 type",
			input:       "(module (import \"\" \"\" (func (param))))",
			expectedErr: "1:35: expected a type in module.import[0].func.param[0]",
		},
		{
			name:        "import func missing param1 type",
			input:       "(module (import \"\" \"\" (func (param i32) (param))))",
			expectedErr: "1:47: expected a type in module.import[0].func.param[1]",
		},
		{
			name:        "import func wrong param0 type",
			input:       "(module (import \"\" \"\" (func (param f65))))",
			expectedErr: "1:36: unknown type: f65 in module.import[0].func.param[0]",
		},
		{
			name:        "import func wrong param1 type",
			input:       "(module (import \"\" \"\" (func (param i32) (param f65))))",
			expectedErr: "1:48: unknown type: f65 in module.import[0].func.param[1]",
		},
		{
			name:        "import func double result",
			input:       "(module (import \"\" \"\" (func (param i32) (result i32) (result i32))))",
			expectedErr: "1:55: redundant result field in module.import[0].func",
		},
		{
			name:        "import func double result type",
			input:       "(module (import \"\" \"\" (func (param i32) (result i32 i32))))",
			expectedErr: "1:53: redundant type in module.import[0].func.result",
		},
		{
			name:        "import func wrong result type",
			input:       "(module (import \"\" \"\" (func (param i32) (result f65))))",
			expectedErr: "1:49: unknown type: f65 in module.import[0].func.result",
		},
		{
			name:        "import func double desc",
			input:       "(module (import \"foo\" \"bar\" (func $main) (func $mein)))",
			expectedErr: "1:43: redundant field: func in module.import[0]",
		},
		{
			name:        "start missing funcidx",
			input:       "(module (start))",
			expectedErr: "1:15: missing funcidx in module.start",
		},
		{
			name:        "start double funcidx",
			input:       "(module (start $main $main))",
			expectedErr: "1:22: redundant funcidx in module.start",
		},
		{
			name:        "double start",
			input:       "(module (start $main) (start $main))",
			expectedErr: "1:24: redundant start in module",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := parseModule([]byte(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
