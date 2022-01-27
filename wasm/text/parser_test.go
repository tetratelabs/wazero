package text

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestParseModule(t *testing.T) {
	f32, i32, i64 := wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeI64
	paramI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32}}
	paramI32I32ResultI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	paramI32I32I32I32ResultI32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}
	paramI32I32I32I32I32I64I32I32ResultI32 := &wasm.FunctionType{
		Params:  []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
		Results: []wasm.ValueType{i32},
	}
	resultI32 := &wasm.FunctionType{Results: []wasm.ValueType{i32}}
	indexZero, indexOne := &index{numeric: wasm.Index(0)}, &index{numeric: wasm.Index(1)}
	localGet0End := []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}

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
			expected: &module{names: &wasm.NameSection{ModuleName: "tools"}},
		},
		{
			name:  "type func one empty",
			input: "(module (type $i32i32_i32 (func (param i32 i32) (result i32))) (type (func)))",
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{},
				},
			},
		},
		{
			name:  "type func mixed param abbreviation", // Verifies we can handle less param fields than Params
			input: "(module (type (func (param i32 i32) (param i32) (param i64) (param f32))))",
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
				},
			},
		},
		{
			name: "type funcs same param types",
			input: `(module
	(type $mul (func (param f32) (param f32) (result f32)))
	(type (func) (; here to ensure sparse indexes work ;))
	(type $add (func (param f32) (param f32) (result f32)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
					{},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
			},
		},
		{
			name: "type func empty after inlined", // ensures the parser was reset properly
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
	(type (func))
)`,
			expected: &module{
				types: []*wasm.FunctionType{
					{}, // module types are always before inlined types
					paramI32I32I32I32ResultI32,
				},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			name:  "import func empty",
			input: "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
			},
		},
		{
			name: "import func redundant",
			input: `(module
	(type (func))
	(import "foo" "bar" (func))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
			},
		},
		{
			name: "import func redundant - late",
			input: `(module
	(import "foo" "bar" (func))
	(type (func))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
			},
		},
		{
			name: "import func redundant - two late", // pun intended
			input: `(module
	(import "foo" "bar" (func))
	(import "baz" "qux" (func))
	(type (func))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
			},
		},
		{
			name: "import func empty after non-empty", // ensures the parser was reset properly
			input: `(module
	(type (func (param i32) (param i32) (param i32) (param i32) (result i32)))
	(import "foo" "bar" (func))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32ResultI32, {}},
				typeUses:    []*typeUse{{typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
			},
		},
		{
			name:  "import func empty twice",
			input: "(module (import \"foo\" \"bar\" (func)) (import \"baz\" \"qux\" (func)))", // ok empty sig
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
			},
		},
		{
			name: "import func inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32) (param i32) (param i32) (param i32) (result i32)))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			name: "import func inlined type - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
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
				types:       []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "fd_write"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			name: "import func inlined type no result",
			input: `(module
	(import "wasi_snapshot_preview1" "proc_exit" (func $runtime.proc_exit (param i32)))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "proc_exit"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.proc_exit"}},
				},
			},
		},
		{
			name:  "import func inlined type no param",
			input: `(module (import "" "" (func (result i32))))`,
			expected: &module{
				types:       []*wasm.FunctionType{resultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{}},
			},
		},
		{
			name: "import func inlined type different param types",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32) (param i32) (param i32) (param i32) (param i32) (param i64) (param i64) (param i32) (param i32) (result i32)))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32I32I64I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "path_open"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "import func inlined type different param types - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{paramI32I32I32I32I32I64I32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "path_open"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "multiple import func different inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func different type - name index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(type $i32i32i32i32_i32 (func (param i32 i32 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (type $i32i32_i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (type $i32i32i32i32_i32)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 88}},
					{typeIndex: &index{numeric: wasm.Index(2), line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func different type - integer index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32)))
	(type (func (param i32 i32 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (type 1)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (type 2)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 88}},
					{typeIndex: &index{numeric: wasm.Index(2), line: 6, col: 76}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
					{module: "wasi_snapshot_preview1", name: "fd_write"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func same inlined type",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (param i32 i32) (result i32)))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexOne}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_get"},
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "multiple import func same type index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (type 1)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 4, col: 76}},
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 88}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_get"},
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "multiple import func same type index - type after import",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (type 1)))
	(type (func (param i32 i32) (result i32)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 3, col: 76}},
					{typeIndex: &index{numeric: wasm.Index(1), line: 4, col: 88}},
				},
				importFuncs: []*importFunc{
					{module: "wasi_snapshot_preview1", name: "args_get"},
					{module: "wasi_snapshot_preview1", name: "args_sizes_get"},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name:  "import func param IDs",
			input: "(module (import \"Math\" \"Mul\" (func $mul (param $x f32) (param $y f32) (result f32))))",
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "Math", name: "Mul"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "mul"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(1), Name: "y"}}},
					},
				},
			},
		},
		{
			name: "import funcs same param types different names",
			input: `(module
	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	(import "Math" "Add" (func $add (param $l f32) (param $r f32) (result f32)))
)`,
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "Math", name: "Mul"}, {module: "Math", name: "Add"}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "mul"}, {Index: wasm.Index(1), Name: "add"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(1), Name: "y"}}},
						{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "l"}, {Index: wasm.Index(1), Name: "r"}}},
					},
				},
			},
		},
		{
			name:  "import func mixed param IDs", // Verifies we can handle less param fields than Params
			input: "(module (import \"\" \"\" (func (param i32 i32) (param $v i32) (param i64) (param $t f32))))",
			expected: &module{
				types: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
				},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "", name: ""}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(2), Name: "v"}, {Index: wasm.Index(4), Name: "t"}}},
					},
				},
			},
		},
		{
			name:  "func empty",
			input: "(module (func))", // ok empty sig
			expected: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant empty type",
			input: `(module
	(type (func))
	(func)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant empty type - late",
			input: `(module
	(func)
	(type (func))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant type - two late", // pun intended
			input: `(module
	(func)
	(func)
	(type (func))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}, {Body: end}},
			},
		},
		{
			name: "func empty after non-empty", // ensures the parser was reset properly
			input: `(module
	(type (func (param i32) (param i32) (param i32) (param i32) (result i32) ))
	(func)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32ResultI32, {}},
				typeUses: []*typeUse{{typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name:  "func empty twice",
			input: "(module (func) (func))", // ok empty sig
			expected: &module{
				types:    []*wasm.FunctionType{{}},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}, {Body: end}},
			},
		},
		{
			name: "func inlined type",
			input: `(module
	(func $runtime.fd_write (param i32) (param i32) (param i32) (param i32) (result i32) local.get 0 )
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			name: "func inlined type - abbreviated",
			input: `(module
	(func $runtime.fd_write (param i32 i32 i32 i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			// Spec says expand abbreviations first. It doesn't explicitly say you can't mix forms.
			// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A0
			name: "func inlined type - mixed abbreviated",
			input: `(module
	(func $runtime.fd_write (param i32) (param i32 i32) (param i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.fd_write"}},
				},
			},
		},
		{
			name: "func inlined type no result",
			input: `(module
	(func $runtime.proc_exit (param i32))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.proc_exit"}},
				},
			},
		},
		{
			name:  "func inlined type no param",
			input: `(module (func (result i32) local.get 0))`,
			expected: &module{
				types:    []*wasm.FunctionType{resultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
			},
		},
		{
			name: "func inlined type different param types",
			input: `(module
	(func $runtime.path_open (param i32) (param i32) (param i32) (param i32) (param i32) (param i64) (param i64) (param i32) (param i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32I32I64I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "func inlined type different param types - abbreviated",
			input: `(module
	(func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32I32I32I32I64I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "multiple func different inlined type",
			input: `(module
	(func $runtime.args_sizes_get (param i32 i32) (result i32) local.get 0)
	(func $runtime.fd_write (param i32 i32 i32 i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexOne}},
				code:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple func different type - name index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type $i32i32_i32 (func (param i32 i32) (result i32) ))
	(type $i32i32i32i32_i32 (func (param i32 i32 i32 i32) (result i32) ))
	(func $runtime.args_sizes_get (type $i32i32_i32) local.get 0)
	(func $runtime.fd_write (type $i32i32i32i32_i32) local.get 0)
)`,
			expected: &module{
				types: []*wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 38}},
					{typeIndex: &index{numeric: wasm.Index(2), line: 6, col: 32}},
				},
				code: []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple func different type - integer index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32) ))
	(type (func (param i32 i32 i32 i32) (result i32) ))
	(func $runtime.args_sizes_get (type 1) local.get 0 )
	(func $runtime.fd_write (type 2) local.get 0 )
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32, paramI32I32I32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 38}},
					{typeIndex: &index{numeric: wasm.Index(2), line: 6, col: 32}},
				},
				code: []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "mixed func same inlined type",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (param i32 i32) (result i32) ))
	(func $runtime.args_sizes_get (param i32 i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses:    []*typeUse{{typeIndex: indexOne}, {typeIndex: indexOne}},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				code:        []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "mixed func same type index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type (func (param i32 i32) (result i32) ))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(func $runtime.args_sizes_get (type 1) local.get 0)
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 4, col: 76}},
					{typeIndex: &index{numeric: wasm.Index(1), line: 5, col: 38}},
				},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				code:        []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "mixed func same type index - type after import",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1)))
	(func $runtime.args_sizes_get (type 1) local.get 0)
	(type (func (param i32 i32) (result i32) ))
)`,
			expected: &module{
				types: []*wasm.FunctionType{{}, paramI32I32ResultI32},
				typeUses: []*typeUse{
					{typeIndex: &index{numeric: wasm.Index(1), line: 3, col: 76}},
					{typeIndex: &index{numeric: wasm.Index(1), line: 4, col: 38}},
				},
				importFuncs: []*importFunc{{module: "wasi_snapshot_preview1", name: "args_get"}},
				code:        []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name:  "func param IDs",
			input: "(module (func $one (param $x i32) (param $y i32) (result i32) local.get 0))",
			expected: &module{
				types:    []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "one"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(1), Name: "y"}}},
					},
				},
			},
		},
		{
			name: "funcs same param types different names",
			input: `(module
	(func (param $x i32) (param $y i32) (result i32) local.get 0)
	(func (param $l i32) (param $r i32) (result i32) local.get 0)
)`,
			expected: &module{
				types:    []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
				typeUses: []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "x"}, {Index: wasm.Index(1), Name: "y"}}},
						{Index: wasm.Index(1), NameMap: wasm.NameMap{{Index: wasm.Index(0), Name: "l"}, {Index: wasm.Index(1), Name: "r"}}},
					},
				},
			},
		},
		{
			name:  "func mixed param IDs", // Verifies we can handle less param fields than Params
			input: "(module (func (param i32 i32) (param $v i32) (param i64) (param $t f32)))",
			expected: &module{
				types:    []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32, i32, i64, f32}}},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code:     []*wasm.Code{{Body: end}},
				names: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{{Index: wasm.Index(2), Name: "v"}, {Index: wasm.Index(4), Name: "t"}}},
					},
				},
			},
		},
		{
			name: "export imported func",
			input: `(module
	(import "foo" "bar" (func $bar))
	(export "bar" (func $bar))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "bar", exportIndex: wasm.Index(0), funcIndex: &index{numeric: wasm.Index(0), line: 3, col: 22}},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "bar"}},
				},
			},
		},
		{
			name: "export imported func - numeric",
			input: `(module
	(import "foo" "bar" (func))
	(export "bar" (func 0))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{{name: "bar", funcIndex: &index{numeric: 0, line: 3, col: 22}}},
			},
		},
		{
			name: "export imported func twice",
			input: `(module
	(import "foo" "bar" (func $bar))
	(export "foo" (func $bar))
	(export "bar" (func $bar))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{numeric: wasm.Index(0), line: 3, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{numeric: wasm.Index(0), line: 4, col: 22}},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: wasm.Index(0), Name: "bar"}},
				},
			},
		},
		{
			name: "export different func",
			input: `(module
	(import "foo" "bar" (func $bar))
	(import "baz" "qux" (func $qux))
	(export "foo" (func $bar))
	(export "bar" (func $qux))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{numeric: wasm.Index(0), line: 4, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{numeric: wasm.Index(1), line: 5, col: 22}},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: wasm.Index(0), Name: "bar"},
						&wasm.NameAssoc{Index: wasm.Index(1), Name: "qux"},
					},
				},
			},
		},
		{
			name: "export different func - numeric",
			input: `(module
	(import "foo" "bar" (func))
	(import "baz" "qux" (func))
	(export "foo" (func 0))
	(export "bar" (func 1))
)`,
			expected: &module{
				types:       []*wasm.FunctionType{{}},
				typeUses:    []*typeUse{{typeIndex: indexZero}, {typeIndex: indexZero}},
				importFuncs: []*importFunc{{module: "foo", name: "bar"}, {module: "baz", name: "qux"}},
				exportFuncs: []*exportFunc{
					{name: "foo", exportIndex: wasm.Index(0), funcIndex: &index{numeric: wasm.Index(0), line: 4, col: 22}},
					{name: "bar", exportIndex: wasm.Index(1), funcIndex: &index{numeric: wasm.Index(1), line: 5, col: 22}},
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
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}},
				importFuncs:   []*importFunc{{name: "hello"}},
				startFunction: &index{numeric: wasm.Index(0), line: 3, col: 9},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "hello"}},
				},
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &module{
				types:         []*wasm.FunctionType{{}},
				typeUses:      []*typeUse{{typeIndex: indexZero}},
				importFuncs:   []*importFunc{{name: "hello"}},
				startFunction: &index{numeric: wasm.Index(0), line: 3, col: 9},
			},
		},
		{
			name: "exported func with instructions",
			input: `(module
	;; from https://github.com/summerwind/the-art-of-webassembly-go/blob/main/chapter1/addint/addint.wat
    (func $addInt ;; TODO: function exports (export "AddInt")
        (param $value_1 i32) (param $value_2 i32)
        (result i32)
        local.get 0 ;; TODO: instruction variables $value_1
        local.get 1 ;; TODO: instruction variables $value_2
        i32.add
    )
    (export "AddInt" (func $addInt))
)`,
			expected: &module{
				types:    []*wasm.FunctionType{paramI32I32ResultI32},
				typeUses: []*typeUse{{typeIndex: indexZero}},
				code: []*wasm.Code{
					{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
				},
				exportFuncs: []*exportFunc{
					{name: "AddInt", exportIndex: wasm.Index(0), funcIndex: &index{numeric: wasm.Index(0), line: 10, col: 28}},
				},
				names: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "addInt"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: wasm.Index(0), NameMap: wasm.NameMap{
							{Index: wasm.Index(0), Name: "value_1"},
							{Index: wasm.Index(1), Name: "value_2"},
						}},
					},
				},
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
			name:        "module double ID",
			input:       "(module $foo $bar)",
			expectedErr: "1:14: redundant ID $bar in module",
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
			name:        "type func missing func",
			input:       "(module (type))",
			expectedErr: "1:14: missing func field in module.type[0]",
		},
		{
			name:        "type func too much func",
			input:       "(module (type (func) (func)))",
			expectedErr: "1:22: unexpected '(' in module.type[0]",
		},
		{
			name:        "type func second ID",
			input:       "(module (type $v_v $v_v func()))",
			expectedErr: "1:20: redundant ID $v_v in module.type[0]",
		},
		{
			name:        "type ID clash",
			input:       "(module (type $1 (func)) (type $1 (func (param i32))))",
			expectedErr: "1:32: duplicate ID $1 in module.type[1]",
		},
		{
			name:        "type func param ID in abbreviation",
			input:       "(module (type (func (param $x i32 i64) ))",
			expectedErr: "1:35: cannot assign IDs to parameters in abbreviated form in module.type[0].func.param[0]",
		},
		{
			name:        "type func ID wrong place",
			input:       "(module (type (func $v_v )))",
			expectedErr: "1:21: unexpected id: $v_v in module.type[0].func",
		},
		{
			name:        "type invalid",
			input:       "(module (type \"0\"))",
			expectedErr: "1:15: unexpected string: \"0\" in module.type[0]",
		},
		{
			name:        "type func invalid",
			input:       "(module (type (func \"0\")))",
			expectedErr: "1:21: unexpected string: \"0\" in module.type[0].func",
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
			name:        "import func double ID",
			input:       "(module (import \"foo\" \"bar\" (func $baz $qux)))",
			expectedErr: "1:40: redundant ID $qux in module.import[0].func",
		},
		{
			name:        "import func clash ID",
			input:       "(module (import \"\" \"\" (func $main)) (import \"\" \"\" (func $main)))",
			expectedErr: "1:57: duplicate ID $main in module.import[1].func",
		},
		{
			name:        "import func param second ID",
			input:       "(module (import \"\" \"\" (func (param $x $y i32) ))",
			expectedErr: "1:39: redundant ID $y in module.import[0].func.param[0]",
		},
		{
			name:        "import func param ID in abbreviation",
			input:       "(module (import \"\" \"\" (func (param $x i32 i64) ))",
			expectedErr: "1:43: cannot assign IDs to parameters in abbreviated form in module.import[0].func.param[0]",
		},
		{
			name:        "import func param ID clash",
			input:       "(module (import \"\" \"\" (func (param $x i32) (param i32) (param $x i32)))_",
			expectedErr: "1:63: duplicate ID $x in module.import[0].func.param[2]",
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
			name:        "import func ID after param",
			input:       "(module (import \"\" \"\" (func (param i32) $main)))",
			expectedErr: "1:41: unexpected id: $main in module.import[0].func",
		},
		{
			name:        "import func ID after result",
			input:       "(module (import \"\" \"\" (func (result i32) $main)))",
			expectedErr: "1:42: unexpected id: $main in module.import[0].func",
		},
		{
			name:        "import func type duplicate index",
			input:       "(module (import \"\" \"\" (func (type 9 2)))",
			expectedErr: "1:37: redundant index in module.import[0].func.type",
		},
		{
			name:        "import func duplicate type",
			input:       "(module (import \"\" \"\" (func (type 9) (type 2)))",
			expectedErr: "1:39: redundant type in module.import[0].func",
		},
		{
			name:        "import func type invalid",
			input:       "(module (import \"\" \"\" (func (type \"0\")))",
			expectedErr: "1:35: unexpected string: \"0\" in module.import[0].func.type",
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
			name:        "func invalid name",
			input:       "(module (func baz)))",
			expectedErr: "1:15: unexpected keyword: baz in module.func[0]",
		},
		{
			name:        "func invalid token",
			input:       "(module (func ($param)))",
			expectedErr: "1:16: unexpected id: $param in module.func[0]",
		},
		{
			name:        "func double param ID",
			input:       "(module (func $baz $qux)))",
			expectedErr: "1:20: redundant ID $qux in module.func[0]",
		},
		{
			name:        "func param ID clash",
			input:       "(module (func (param $x i32) (param i32) (param $x i32)))",
			expectedErr: "1:49: duplicate ID $x in module.func[0].param[2]",
		},
		{
			name:        "func param second ID",
			input:       "(module (func (param $x $y i32) ))",
			expectedErr: "1:25: redundant ID $y in module.func[0].param[0]",
		},
		{
			name:        "func param ID in abbreviation",
			input:       "(module (func (param $x i32 i64) ))",
			expectedErr: "1:29: cannot assign IDs to parameters in abbreviated form in module.func[0].param[0]",
		},
		{
			name:        "func missing param0 type",
			input:       "(module (func (param)))",
			expectedErr: "1:21: expected a type in module.func[0].param[0]",
		},
		{
			name:        "func missing param1 type",
			input:       "(module (func (param i32) (param)))",
			expectedErr: "1:33: expected a type in module.func[0].param[1]",
		},
		{
			name:        "func wrong param0 type",
			input:       "(module (func (param f65)))",
			expectedErr: "1:22: unknown type: f65 in module.func[0].param[0]",
		},
		{
			name:        "func wrong param1 type",
			input:       "(module (func (param i32) (param f65)))",
			expectedErr: "1:34: unknown type: f65 in module.func[0].param[1]",
		},
		{
			name:        "func double result",
			input:       "(module (func (param i32) (result i32) (result i32)))",
			expectedErr: "1:41: result declared out of order in module.func[0]",
		},
		{
			name:        "func double result type",
			input:       "(module (func (param i32) (result i32 i32)))",
			expectedErr: "1:39: redundant type in module.func[0].result",
		},
		{
			name:        "func wrong result type",
			input:       "(module (func (param i32) (result f65)))",
			expectedErr: "1:35: unknown type: f65 in module.func[0].result",
		},
		{
			name:        "func wrong no param type",
			input:       "(module (func (param)))",
			expectedErr: "1:21: expected a type in module.func[0].param[0]",
		},
		{
			name:        "func no result type",
			input:       "(module (func (param i32) (result)))",
			expectedErr: "1:34: expected a type in module.func[0].result",
		},
		{
			name:        "func wrong param token",
			input:       "(module (func (param () )))",
			expectedErr: "1:22: unexpected '(' in module.func[0].param[0]",
		},
		{
			name:        "func wrong result token",
			input:       "(module (func (result () )))",
			expectedErr: "1:23: unexpected '(' in module.func[0].result",
		},
		{
			name:        "func ID after param",
			input:       "(module (func (param i32) $main)))",
			expectedErr: "1:27: unexpected id: $main in module.func[0]",
		},
		{
			name:        "clash on func ID",
			input:       "(module (func $main) (func $main)))",
			expectedErr: "1:28: duplicate ID $main in module.func[1]",
		},
		{
			name:        "func ID clashes with import func ID",
			input:       "(module (import \"\" \"\" (func $main)) (func $main)))",
			expectedErr: "1:43: duplicate ID $main in module.func[1]",
		},
		{
			name:        "import func ID clashes with func ID",
			input:       "(module (func $main) (import \"\" \"\" (func $main)))",
			expectedErr: "1:42: duplicate ID $main in module.import[1].func",
		},
		{
			name:        "func ID after result",
			input:       "(module (func (result i32) $main)))",
			expectedErr: "1:28: unexpected id: $main in module.func[0]",
		},
		{
			name:        "func type duplicate index",
			input:       "(module (func (type 9 2)))",
			expectedErr: "1:23: redundant index in module.func[0].type",
		},
		{
			name:        "func duplicate type",
			input:       "(module (func (type 9) (type 2)))",
			expectedErr: "1:25: redundant type in module.func[0]",
		},
		{
			name:        "func type invalid",
			input:       "(module (func (type \"0\")))",
			expectedErr: "1:21: unexpected string: \"0\" in module.func[0].type",
		},
		{
			name:        "func param after result",
			input:       "(module (func (result i32) (param i32)))",
			expectedErr: "1:29: param declared out of order in module.func[0]",
		},
		{
			name:        "export double name",
			input:       "(module (export \"PI\" \"PI\" (func main)))",
			expectedErr: "1:22: redundant name: PI in module.export[0]",
		},
		{
			name:        "export wrong name",
			input:       "(module (export PI (func $main)))",
			expectedErr: "1:17: unexpected reserved: PI in module.export[0]",
		},
		{
			name:        "export func missing index",
			input:       "(module (export \"PI\" (func)))",
			expectedErr: "1:27: missing index in module.export[0].func",
		},
		{
			name:        "export func double index",
			input:       "(module (export \"PI\" (func $main $main)))",
			expectedErr: "1:34: redundant index in module.export[0].func",
		},
		{
			name:        "export func wrong index",
			input:       "(module (export \"PI\" (func main)))",
			expectedErr: "1:28: unexpected keyword: main in module.export[0].func",
		},
		{
			name: "export func points out of range",
			input: `(module
	(import "" "hello" (func))
	(export "PI" (func 1))
)`,
			expectedErr: "3:21: index 1 is out of range [0..0] in module.exports[0].func",
		},
		{
			name:        "export func points nowhere",
			input:       "(module (export \"PI\" (func $main)))",
			expectedErr: "1:28: unknown ID $main in module.exports[0].func",
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
