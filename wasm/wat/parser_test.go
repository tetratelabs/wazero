package wat

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

// example has a work-in-progress of supported functionality, used primarily for benchmarking. This includes:
// * module and function names
// * explicit, and inlined type definitions (including anonymous)
// * start function
//
// NOTE: this is different from exampleWat because the parser doesn't yet support all features
var example = []byte(`(module $example
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (type $i32i32_i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
	(type (func))
	(import "" "hello" (func $hello (type 1)))
	(start $hello)
)`)

func TestParseModule(t *testing.T) {
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64
	paramI32 := &typeFunc{params: []wasm.ValueType{i32}}
	paramI32I32ResultI32 := &typeFunc{params: []wasm.ValueType{i32, i32}, result: i32}
	paramI32I32I32I32ResultI32 := &typeFunc{params: []wasm.ValueType{i32, i32, i32, i32}, result: i32}
	paramI32I32I32I32I32I64I32I32ResultI32 := &typeFunc{
		params: []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
		result: i32,
	}
	resultI32 := &typeFunc{result: i32}
	indexZero, indexOne := &index{numeric: 0}, &index{numeric: 1}

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
			name:  "type one empty",
			input: "(module (type $i32i32_i32 (func (param i32 i32) (result i32))) (type (func)))",
			expected: &module{
				typeFuncs: []*typeFunc{
					{name: "$i32i32_i32", params: []wasm.ValueType{i32, i32}, result: i32},
					typeFuncEmpty,
				},
			},
		},
		{
			name: "type empty after inlined", // ensures the parser was reset properly
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
	(type (func))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{
					typeFuncEmpty, // module types are always before inlined types
					paramI32I32I32I32ResultI32,
				},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: indexOne},
				},
			},
		},
		{
			name:  "import func empty",
			input: "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &module{
				typeFuncs:   []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{{module: "foo", name: "bar", typeIndex: indexZero}},
			},
		},
		{
			name: "import func empty after non-empty", // ensures the parser was reset properly
			input: `(module
	(type (func (param i32) (param i32) (param i32) (param i32) (result i32)))
	(import "foo" "bar" (func))
)`,
			expected: &module{
				typeFuncs:   []*typeFunc{paramI32I32I32I32ResultI32, typeFuncEmpty},
				importFuncs: []*importFunc{{module: "foo", name: "bar", typeIndex: indexOne}},
			},
		},
		{
			name:  "import func empty twice",
			input: "(module (import \"foo\" \"bar\" (func)) (import \"baz\" \"qux\" (func)))", // ok empty sig
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "foo", name: "bar", typeIndex: indexZero},
					{importIndex: 1, module: "baz", name: "qux", typeIndex: indexZero},
				},
			},
		},
		{
			name: "import func inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32) (param i32) (param i32) (param i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: indexZero},
				},
			},
		},
		{
			name: "import func inlined type - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: indexZero},
				},
			},
		},
		{
			// Spec says expand abbreviations first. It doesn't explicitly say you can't mix forms.
			// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A0
			name: "import func inlined type - mixed abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32) (param i32 i32) (param i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: indexZero},
				},
			},
		},
		{
			name: "import func inlined type no result",
			input: `(module
	(import "wasi_snapshot_preview1" "proc_exit" (func $runtime.proc_exit (param i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "proc_exit", funcName: "$runtime.proc_exit",
						typeIndex: indexZero},
				},
			},
		},
		{
			name:  "import func inlined type no param",
			input: `(module (import "" "" (func (result i32))))`,
			expected: &module{
				typeFuncs:   []*typeFunc{resultI32},
				importFuncs: []*importFunc{{typeIndex: indexZero}},
			},
		},
		{
			name: "import func inlined type different param types",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32) (param i32) (param i32) (param i32) (param i32) (param i64) (param i64) (param i32) (param i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32I32I32I32I32I64I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "path_open", funcName: "$runtime.path_open",
						typeIndex: indexZero},
				},
			},
		},
		{
			name: "import func inlined type different param types - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{paramI32I32I32I32I32I64I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "path_open", funcName: "$runtime.path_open",
						typeIndex: indexZero},
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
				typeFuncs: []*typeFunc{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: indexZero},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: indexOne},
				},
			},
		},
		{
			name: "multiple import func different type - name index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(type $i32i32i32i32_i32 (func (param i32 i32 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (type $i32i32_i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (type $i32i32i32i32_i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{
					typeFuncEmpty,
					{name: "$i32i32_i32", params: []wasm.ValueType{i32, i32}, result: i32},
					{name: "$i32i32i32i32_i32", params: []wasm.ValueType{i32, i32, i32, i32}, result: i32},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
			},
		},
		{
			name: "multiple import func different type - integer index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32)))
	(type (func (param i32 i32 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (type 1)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (type 2)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 6, col: 76}},
				},
			},
		},
		{
			name: "multiple import func same inlined type",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (param i32 i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "$runtime.args_get",
						typeIndex: indexOne},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: indexOne},
				},
			},
		},
		{
			name: "multiple import func same type index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (type 1)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "$runtime.args_get",
						typeIndex: &index{numeric: 1, line: 4, col: 76}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 5, col: 86}},
				},
			},
		},
		{
			name: "multiple import func same type index - type after import",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(import "wasi_snapshot_preview1" "arg_sizes_get" (func $runtime.arg_sizes_get (type 1)))
	(type (func (param i32 i32) (result i32)))
)`,
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty, paramI32I32ResultI32},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "args_get", funcName: "$runtime.args_get",
						typeIndex: &index{numeric: 1, line: 3, col: 76}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: &index{numeric: 1, line: 4, col: 86}},
				},
			},
		},
		{
			name: "start imported function by name",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &module{
				typeFuncs:     []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", funcName: "$hello", typeIndex: indexZero}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &module{
				typeFuncs:     []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeIndex: indexZero}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
		},
		{
			name:  "example",
			input: string(example),
			expected: &module{
				name: "$example",
				typeFuncs: []*typeFunc{
					{name: "$i32i32_i32", params: []wasm.ValueType{i32, i32}, result: i32},
					typeFuncEmpty, // Note: inlined types come after explicit ones even if the latter are defined later
					{params: []wasm.ValueType{i32, i32, i32, i32}, result: i32},
				},
				importFuncs: []*importFunc{
					{importIndex: 0, module: "wasi_snapshot_preview1", name: "arg_sizes_get", funcName: "$runtime.arg_sizes_get",
						typeIndex: &index{numeric: 0, line: 3, col: 86}},
					{importIndex: 1, module: "wasi_snapshot_preview1", name: "fd_write", funcName: "$runtime.fd_write",
						typeIndex: &index{numeric: 2, line: 0, col: 0}},
					{importIndex: 2, module: "", name: "hello", funcName: "$hello",
						typeIndex: &index{numeric: 1, line: 6, col: 40}},
				},
				startFunction: &index{numeric: 2, line: 7, col: 9},
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
			name:        "forgot parens",
			input:       "module",
			expectedErr: "1:1: expected '(', but found keyword: module",
		},
		{
			name:        "no module",
			input:       "()",
			expectedErr: "1:2: expected field, but found )",
		},
		{
			name:        "double module",
			input:       "(module) (module)",
			expectedErr: "1:10: unexpected trailing characters: (",
		},
		{
			name:        "module invalid name",
			input:       "(module test)", // must start with $
			expectedErr: "1:9: unexpected keyword: test in module",
		},
		{
			name:        "module invalid field",
			input:       "(module (test))",
			expectedErr: "1:10: unexpected field: test in module",
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
			name:        "module name after import",
			input:       "(module (import \"\" \"\" (func) $Math)",
			expectedErr: "1:30: unexpected id: $Math in module.import[0]",
		},
		{
			name:        "type missing func",
			input:       "(module (type))",
			expectedErr: "1:14: missing func field in module.type[0]",
		},
		{
			name:        "type too much func",
			input:       "(module (type (func) (func)))",
			expectedErr: "1:22: unexpected '(' in module.type[0]",
		},
		{
			name:        "type second name",
			input:       "(module (type $v_v $v_v func()))",
			expectedErr: "1:20: unexpected id: $v_v in module.type[0]",
		},
		{
			name:        "type name wrong place",
			input:       "(module (type (func $v_v )))",
			expectedErr: "1:21: unexpected id: $v_v in module.type[0].func",
		},
		{
			name:        "import missing module",
			input:       "(module (import))",
			expectedErr: "1:16: missing module and name in module.import[0]",
		},
		{
			name:        "import with desc, but no module",
			input:       "(module (import (func))",
			expectedErr: "1:17: missing module and name in module.import[0]",
		},
		{
			name:        "import missing name",
			input:       "(module (import \"\"))",
			expectedErr: "1:19: missing name in module.import[0]",
		},
		{
			name:        "import name not a string",
			input:       "(module (import \"\" 0))",
			expectedErr: "1:20: unexpected uN: 0 in module.import[0]",
		},
		{
			name:        "import with desc, no name",
			input:       "(module (import \"\" (func)))",
			expectedErr: "1:20: missing name in module.import[0]",
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
			name:        "import func invalid token",
			input:       "(module (import \"foo\" \"bar\" (func ($param))))",
			expectedErr: "1:36: unexpected id: $param in module.import[0].func",
		},
		{
			name:        "import func double name",
			input:       "(module (import \"foo\" \"bar\" (func $baz $qux)))",
			expectedErr: "1:40: redundant name: $qux in module.import[0].func",
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
			expectedErr: "1:54: unexpected '(' in module.import[0].func",
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
			name:        "import func wrong no param type",
			input:       "(module (import \"\" \"\" (func (param))))",
			expectedErr: "1:35: expected a type in module.import[0].func.param[0]",
		},
		{
			name:        "import func no result type",
			input:       "(module (import \"\" \"\" (func (param i32) (result))))",
			expectedErr: "1:48: expected a type in module.import[0].func.result",
		},
		{
			name:        "import func wrong param token",
			input:       "(module (import \"\" \"\" (func (param () ))))",
			expectedErr: "1:36: unexpected '(' in module.import[0].func.param[0]",
		},
		{
			name:        "import func wrong result token",
			input:       "(module (import \"\" \"\" (func (result () ))))",
			expectedErr: "1:37: unexpected '(' in module.import[0].func.result",
		},
		{
			name:        "import func name after param",
			input:       "(module (import \"\" \"\" (func (param i32) $main)))",
			expectedErr: "1:41: unexpected id: $main in module.import[0].func",
		},
		{
			name:        "import func name after result",
			input:       "(module (import \"\" \"\" (func (result i32) $main)))",
			expectedErr: "1:42: unexpected id: $main in module.import[0].func",
		},
		{
			name:        "import func param after result",
			input:       "(module (import \"\" \"\" (func (result i32) (param i32))))",
			expectedErr: "1:42: unexpected '(' in module.import[0].func",
		},
		{
			name:        "import func double desc",
			input:       "(module (import \"foo\" \"bar\" (func $main) (func $mein)))",
			expectedErr: "1:42: unexpected '(' in module.import[0]",
		},
		{
			name:        "start missing index",
			input:       "(module (start))",
			expectedErr: "1:15: missing index in module.start",
		},
		{
			name:        "start double index",
			input:       "(module (start $main $main))",
			expectedErr: "1:22: redundant index in module.start",
		},
		{
			name:        "double start",
			input:       "(module (start $main) (start $main))",
			expectedErr: "1:24: redundant start in module",
		},
		{
			name:        "wrong start",
			input:       "(module (start main))",
			expectedErr: "1:16: unexpected keyword: main in module.start",
		},
		{
			name: "start points out of range",
			input: `(module
	(import "" "hello" (func))
	(start 1)
)`,
			expectedErr: "3:9: index 1 is out of range [0..0] in module.start",
		},
		{
			name:        "start points nowhere",
			input:       "(module (start $main))",
			expectedErr: "1:16: unknown ID $main in module.start",
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
