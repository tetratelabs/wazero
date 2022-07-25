package internal

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestDecodeModule(t *testing.T) {
	zero := uint32(0)
	localGet0End := []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}

	tests := []struct {
		name, input string
		expected    *wasm.Module
	}{
		{
			name:     "empty",
			input:    "(module)",
			expected: &wasm.Module{},
		},
		{
			name:     "only name",
			input:    "(module $tools)",
			expected: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "tools"}},
		},
		{
			name: "type funcs same param types",
			input: `(module
	(type (func (param i32) (param i64) (result i32)))
	(type (func) (; here to ensure sparse indexes work ;))
	(type (func (param i32) (param i64) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i64_i32, v_v, i32i64_i32},
			},
		},
		{
			name: "type func empty after inlined", // ensures the parser was reset properly
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32 i32 i32 i32) (result i32)))
	(type (func))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					v_v, // module types are always before inlined types
					i32i32i32i32_i32,
				},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "fd_write",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 1,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name:  "type func multiple abbreviated results",
			input: "(module (type (func (param i32 i32) (result i32 i32))))",
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}},
				},
			},
		},
		{
			name:  "type func multiple results",
			input: "(module (type (func (param i32) (param i32) (result i32) (result i32))))",
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}},
				},
			},
		},
		{
			name:  "import func empty",
			input: "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func redundant",
			input: `(module
	(type (func))
	(import "foo" "bar" (func))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func redundant - late",
			input: `(module
	(import "foo" "bar" (func))
	(type (func))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func redundant - two late", // pun intended
			input: `(module
	(import "foo" "bar" (func))
	(import "baz" "qux" (func))
	(type (func))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}, {
					Module: "baz", Name: "qux",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func empty after non-empty", // ensures the parser was reset properly
			input: `(module
	(type (func (param i32) (param i32) (param i32) (param i32) (result i32)))
	(import "foo" "bar" (func))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32_i32, {}},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 1,
				}},
			},
		},
		{
			name:  "import func empty twice",
			input: "(module (import \"foo\" \"bar\" (func)) (import \"baz\" \"qux\" (func)))", // ok empty sig
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}, {
					Module: "baz", Name: "qux",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32) (param i32) (param i32) (param i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "fd_write",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name: "import func inlined type - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "fd_write",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name: "func call - index - after import",
			input: `(module
			(import "" "" (func))
			(func)
			(func call 1)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				ImportSection:   []*wasm.Import{{Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0, 0},
				CodeSection: []*wasm.Code{
					{Body: end}, {Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}},
				},
			},
		},
		{
			name: "func sign-extension",
			input: `(module
			(func (param i64) (result i64) local.get 0 i64.extend16_s)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i64_i64},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{
					{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeI64Extend16S, wasm.OpcodeEnd}},
				},
			},
		},
		{
			name: "func nontrapping-float-to-int-conversions",
			input: `(module
			(func (param f32) (result i32) local.get 0 i32.trunc_sat_f32_s)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{f32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
					wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32S,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			// Spec says expand abbreviations first. It doesn't explicitly say you can't mix forms.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A0
			name: "import func inlined type - mixed abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32) (param i32 i32) (param i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "fd_write",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name: "import func inlined type no result",
			input: `(module
	(import "wasi_snapshot_preview1" "proc_exit" (func $wasi.proc_exit (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32_v},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "proc_exit",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.proc_exit"}},
				},
			},
		},
		{
			name:  "import func inlined type no param",
			input: `(module (import "" "" (func (result i32))))`,
			expected: &wasm.Module{
				TypeSection:   []*wasm.FunctionType{v_i32},
				ImportSection: []*wasm.Import{{Type: wasm.ExternTypeFunc, DescFunc: 0}},
			},
		},
		{
			name: "import func inlined type different param types",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $wasi.path_open (param i32) (param i32) (param i32) (param i32) (param i32) (param i64) (param i64) (param i32) (param i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32i32i64i64i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "path_open",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.path_open"}},
				},
			},
		},
		{
			name: "import func inlined type different param types - abbreviated",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $wasi.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32i32i32i32i64i64i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "path_open",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.path_open"}},
				},
			},
		},
		{
			name: "multiple import func different inlined type",
			input: `(module
			(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (param i32 i32) (result i32)))
			(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32 i32 i32 i32) (result i32)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i32_i32, i32i32i32i32_i32},
				ImportSection: []*wasm.Import{{
					Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}, {
					Module: "wasi_snapshot_preview1", Name: "fd_write",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 1,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "wasi.args_sizes_get"},
						&wasm.NameAssoc{Index: 1, Name: "wasi.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func different type - ID index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(type $i32i32i32i32_i32 (func (param i32 i32 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (type $i32i32_i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (type $i32i32i32i32_i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					v_v,
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "fd_write",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 2,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "wasi.args_sizes_get"},
						{Index: 1, Name: "wasi.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func different type - numeric index",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(type (func (param i32 i32) (result i32)))
			(type (func (param i32 i32 i32 i32) (result i32)))
			(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (type 1)))
			(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (type 2)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32, i32i32i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "fd_write",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 2,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "wasi.args_sizes_get"},
						&wasm.NameAssoc{Index: 1, Name: "wasi.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple import func same inlined type",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(import "wasi_snapshot_preview1" "environ_get" (func $wasi.environ_get (param i32 i32) (result i32)))
			(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (param i32 i32) (result i32)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "environ_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "wasi.environ_get"},
						&wasm.NameAssoc{Index: 1, Name: "wasi.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "multiple import func same type index",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(type (func (param i32 i32) (result i32)))
			(import "wasi_snapshot_preview1" "environ_get" (func $wasi.environ_get (type 1)))
			(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (type 1)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "environ_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "wasi.environ_get"},
						&wasm.NameAssoc{Index: 1, Name: "wasi.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "multiple import func same type index - type after import",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(import "wasi_snapshot_preview1" "environ_get" (func $wasi.environ_get (type 1)))
			(import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (type 1)))
			(type (func (param i32 i32) (result i32)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "environ_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "args_sizes_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "wasi.environ_get"},
						&wasm.NameAssoc{Index: 1, Name: "wasi.args_sizes_get"},
					},
				},
			},
		},
		{
			name:  "import func param IDs",
			input: "(module (import \"Math\" \"Mul\" (func $mul (param $x i32) (param $y i64) (result i32))))",
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i64_i32},
				ImportSection: []*wasm.Import{{
					Module: "Math", Name: "Mul",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "mul"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: 0, Name: "x"}, {Index: 1, Name: "y"}}},
					},
				},
			},
		},
		{
			name: "import funcs same param types different names",
			input: `(module
			(import "Math" "Mul" (func $mul (param $x i32) (param $y i64) (result i32)))
			(import "Math" "Add" (func $add (param $l i32) (param $r i64) (result i32)))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{i32i64_i32},
				ImportSection: []*wasm.Import{{
					Module: "Math", Name: "Mul",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}, {
					Module: "Math", Name: "Add",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "mul"}, {Index: 1, Name: "add"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: 0, Name: "x"}, {Index: 1, Name: "y"}}},
						{Index: 1, NameMap: wasm.NameMap{{Index: 0, Name: "l"}, {Index: 1, Name: "r"}}},
					},
				},
			},
		},
		{
			name:  "import func mixed param IDs", // Verifies we can handle less param fields than Params
			input: "(module (import \"\" \"\" (func (param i32 i32) (param $v i32) (param i64) (param $t f32))))",
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
				},
				ImportSection: []*wasm.Import{{Type: wasm.ExternTypeFunc, DescFunc: 0}},
				NameSection: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: wasm.Index(2), Name: "v"}, {Index: wasm.Index(4), Name: "t"}}},
					},
				},
			},
		},
		{
			name: "multiple import func with different inlined type",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(import "wasi_snapshot_preview1" "path_open" (func $wasi.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					v_v,
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "path_open",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "wasi_snapshot_preview1", Name: "fd_write",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 2,
					},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "wasi.path_open"},
						{Index: 1, Name: "wasi.fd_write"},
					},
				},
			},
		},
		{
			name: "import func inlined type match - index",
			input: `(module
	(type $i32 (func (param i32)))
	(import "foo" "bar" (func (type 0) (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func inlined type match - index - late",
			input: `(module
	(import "foo" "bar" (func (type 0) (param i32)))
	(type $i32 (func (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func inlined type match - ID",
			input: `(module
	(type $i32 (func (param i32)))
	(import "foo" "bar" (func (type $i32) (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name: "import func inlined type match - ID - late",
			input: `(module
	(import "foo" "bar" (func (type $i32) (param i32)))
	(type $i32 (func (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				ImportSection: []*wasm.Import{{
					Module: "foo", Name: "bar",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
			},
		},
		{
			name:  "import func multiple abbreviated results",
			input: `(module (import "misc" "swap" (func $swap (param i32 i32) (result i32 i32))))`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}}},
				ImportSection: []*wasm.Import{{
					Module: "misc", Name: "swap",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "swap"}},
				},
			},
		},
		{
			name:  "func empty",
			input: "(module (func))", // ok empty sig
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant empty type",
			input: `(module
			(type (func))
			(func)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant empty type - late",
			input: `(module
			(func)
			(type (func))
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name: "func redundant type - two late", // pun intended
			input: `(module
			(func)
			(func)
			(type (func))
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0},
				CodeSection:     []*wasm.Code{{Body: end}, {Body: end}},
			},
		},
		{
			name: "func empty after non-empty", // ensures the parser was reset properly
			input: `(module
			(type (func (param i32) (param i32) (param i32) (param i32) (result i32) ))
			(func)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32_i32, {}},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: end}},
			},
		},
		{
			name:  "func empty twice",
			input: "(module (func) (func))", // ok empty sig
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0},
				CodeSection:     []*wasm.Code{{Body: end}, {Body: end}},
			},
		},
		{
			name: "func inlined type",
			input: `(module
			(func $wasi.fd_write (param i32) (param i32) (param i32) (param i32) (result i32) local.get 0 )
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name: "func inlined type - abbreviated",
			input: `(module
			(func $wasi.fd_write (param i32 i32 i32 i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			// Spec says expand abbreviations first. It doesn't explicitly say you can't mix forms.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A0
			name: "func inlined type - mixed abbreviated",
			input: `(module
			(func $wasi.fd_write (param i32) (param i32 i32) (param i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "wasi.fd_write"}},
				},
			},
		},
		{
			name: "func inlined type no result",
			input: `(module
			(func $runtime.proc_exit (param i32))
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "runtime.proc_exit"}},
				},
			},
		},
		{
			name:  "func inlined type no param",
			input: `(module (func (result i32) local.get 0))`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
			},
		},
		{
			name: "func inlined type different param types",
			input: `(module
			(func $runtime.path_open (param i32) (param i32) (param i32) (param i32) (param i32) (param i64) (param i64) (param i32) (param i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32i32i64i64i32i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "func inlined type different param types - abbreviated",
			input: `(module
			(func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32i32i32i32i64i64i32i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "runtime.path_open"}},
				},
			},
		},
		{
			name: "multiple func different inlined type",
			input: `(module
			(func $runtime.args_sizes_get (param i32 i32) (result i32) local.get 0)
			(func $runtime.fd_write (param i32 i32 i32 i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32i32_i32, i32i32i32i32_i32},
				FunctionSection: []wasm.Index{0, 1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple func with different inlined type",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32) local.get 0)
	(func $runtime.fd_write (param i32 i32 i32 i32) (result i32) local.get 0)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					v_v,
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{1, 2},
				CodeSection:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "runtime.path_open"},
						{Index: 1, Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple func different type - ID index",
			input: `(module
	(type (func) (; ensures no false match on index 0 ;))
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(type $i32i32i32i32_i32 (func (param i32 i32 i32 i32) (result i32)))
	(func $runtime.args_sizes_get (type $i32i32_i32) local.get 0)
	(func $runtime.fd_write (type $i32i32i32i32_i32) local.get 0)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					v_v,
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{1, 2},
				CodeSection:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "runtime.args_sizes_get"},
						{Index: 1, Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "multiple func different type - numeric index",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(type (func (param i32 i32) (result i32) ))
			(type (func (param i32 i32 i32 i32) (result i32) ))
			(func $runtime.args_sizes_get (type 1) local.get 0 )
			(func $runtime.fd_write (type 2) local.get 0 )
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v, i32i32_i32, i32i32i32i32_i32},
				FunctionSection: []wasm.Index{1, 2},
				CodeSection:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_sizes_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.fd_write"},
					},
				},
			},
		},
		{
			name: "func inlined type match - index",
			input: `(module
	(type $i32 (func (param i32)))
	(func (type 0) (param i32))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{codeEnd},
			},
		},
		{
			name: "func inlined type match - index - late",
			input: `(module
	(func (type 0) (param i32))
	(type $i32 (func (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{codeEnd},
			},
		},
		{
			name: "func inlined type match - ID",
			input: `(module
	(type $i32 (func (param i32)))
	(func (type $i32) (param i32))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{codeEnd},
			},
		},
		{
			name: "func inlined type match - ID - late",
			input: `(module
	(func (type $i32) (param i32))
	(type $i32 (func (param i32)))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{codeEnd},
			},
		},
		{
			name: "mixed func same inlined type",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (param i32 i32) (result i32) ))
			(func $runtime.args_sizes_get (param i32 i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.args_sizes_get"},
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
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.args_sizes_get"},
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
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "mixed func signature needs match",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(type (func (param i32 i32) (result i32) ))
			(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1) (param i32 i32) (result i32)))
			(func $runtime.args_sizes_get (type 1) (param i32 i32) (result i32) local.get 0)
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name: "mixed func signature needs match - type after import",
			input: `(module
			(type (func) (; ensures no false match on index 0 ;))
			(import "wasi_snapshot_preview1" "args_get" (func $runtime.args_get (type 1) (param i32 i32) (result i32)))
			(func $runtime.args_sizes_get (type 1) (param i32 i32) (result i32) local.get 0)
			(type (func (param i32 i32) (result i32) ))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v, i32i32_i32},
				ImportSection: []*wasm.Import{
					{
						Module: "wasi_snapshot_preview1", Name: "args_get",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					},
				},
				FunctionSection: []wasm.Index{1},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						&wasm.NameAssoc{Index: 0, Name: "runtime.args_get"},
						&wasm.NameAssoc{Index: 1, Name: "runtime.args_sizes_get"},
					},
				},
			},
		},
		{
			name:  "func param IDs",
			input: "(module (func $one (param $x i32) (param $y i32) (result i32) local.get 0))",
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "one"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: 0, Name: "x"}, {Index: 1, Name: "y"}}},
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
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
				FunctionSection: []wasm.Index{0, 0},
				CodeSection:     []*wasm.Code{{Body: localGet0End}, {Body: localGet0End}},
				NameSection: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: 0, Name: "x"}, {Index: 1, Name: "y"}}},
						{Index: 1, NameMap: wasm.NameMap{{Index: 0, Name: "l"}, {Index: 1, Name: "r"}}},
					},
				},
			},
		},
		{
			name:  "func mixed param IDs", // Verifies we can handle less param fields than Params
			input: "(module (func (param i32 i32) (param $v i32) (param i64) (param $t f32)))",
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32, i32, i64, f32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				NameSection: &wasm.NameSection{
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{{Index: wasm.Index(2), Name: "v"}, {Index: wasm.Index(4), Name: "t"}}},
					},
				},
			},
		},
		{
			name:  "func multiple abbreviated results",
			input: "(module (func $swap (param i32 i32) (result i32 i32) local.get 1 local.get 0))",
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0x01, wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}}},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: wasm.Index(0), Name: "swap"}},
				},
			},
		},
		{
			name: "func call - index",
			input: `(module
			(func)
			(func)
			(func call 1)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0, 0},
				CodeSection: []*wasm.Code{
					{Body: end}, {Body: end}, {Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}},
				},
			},
		},
		{
			name: "func call - index - late",
			input: `(module
			(func)
			(func call 1)
			(func)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0, 0},
				CodeSection: []*wasm.Code{
					{Body: end}, {Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}}, {Body: end},
				},
			},
		},
		{
			name: "func call - ID",
			input: `(module
			(func)
			(func $main)
			(func call $main)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0, 0},
				CodeSection: []*wasm.Code{
					{Body: end}, {Body: end}, {Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 1, Name: "main"}},
				},
			},
		},
		{
			name: "func call - ID - late",
			input: `(module
			(func)
			(func call $main)
			(func $main)
		)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0, 0},
				CodeSection: []*wasm.Code{
					{Body: end}, {Body: []byte{wasm.OpcodeCall, 0x02, wasm.OpcodeEnd}}, {Body: end},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 2, Name: "main"}},
				},
			},
		},
		{
			name:  "memory",
			input: "(module (memory 1))",
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: wasm.MemoryLimitPages},
			},
		},
		{
			name:  "memory ID",
			input: "(module (memory $mem 1))",
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: wasm.MemoryLimitPages},
			},
		},
		{
			name: "export imported func",
			input: `(module
	(import "foo" "bar" (func $bar))
	(export "bar" (func $bar))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{
					{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0},
				},
				ExportSection: []*wasm.Export{
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{FunctionNames: wasm.NameMap{{Index: 0, Name: "bar"}}},
			},
		},
		{
			name: "export imported func - numeric",
			input: `(module
			(import "foo" "bar" (func))
			(export "bar" (func 0))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{
					{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0},
				},
				ExportSection: []*wasm.Export{
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 0},
				},
			},
		},
		{
			name: "export imported func twice",
			input: `(module
			(import "foo" "bar" (func $bar))
			(export "foo" (func $bar))
			(export "bar" (func $bar))
		)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{
					{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0},
				},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "bar"}},
				},
			},
		},
		{
			name: "export different func",
			input: `(module
	(import "foo" "bar" (func $bar))
	(func $qux)
	(export "foo" (func $bar))
	(export "bar" (func $qux))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				ImportSection:   []*wasm.Import{{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "bar"},
						{Index: 1, Name: "qux"},
					},
				},
			},
		},
		{
			name: "export different func - late",
			input: `(module
	(export "foo" (func $bar))
	(export "bar" (func $qux))
	(import "foo" "bar" (func $bar))
	(func $qux)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				ImportSection:   []*wasm.Import{{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{
						{Index: 0, Name: "bar"},
						{Index: 1, Name: "qux"},
					},
				},
			},
		},
		{
			name: "export different func - numeric",
			input: `(module
	(import "foo" "bar" (func))
	(func)
	(export "foo" (func 0))
	(export "bar" (func 1))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				ImportSection:   []*wasm.Import{{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 1},
				},
			},
		},
		{
			name: "export different func - numeric - late",
			input: `(module
	(export "foo" (func 0))
	(export "bar" (func 1))
	(import "foo" "bar" (func))
	(func)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				ImportSection:   []*wasm.Import{{Module: "foo", Name: "bar", Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "bar", Type: wasm.ExternTypeFunc, Index: 1},
				},
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
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{
					{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
				},
				ExportSection: []*wasm.Export{
					{Name: "AddInt", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "addInt"}},
					LocalNames: wasm.IndirectNameMap{
						{Index: 0, NameMap: wasm.NameMap{
							{Index: 0, Name: "value_1"},
							{Index: 1, Name: "value_2"},
						}},
					},
				},
			},
		},
		{
			name: "export memory - numeric",
			input: `(module
	(memory 0)
	(export "foo" (memory 0))
)`,
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 0, Max: wasm.MemoryLimitPages},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "export memory - numeric - late",
			input: `(module
	(export "foo" (memory 0))
	(memory 0)
)`,
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 0, Max: wasm.MemoryLimitPages},
				ExportSection: []*wasm.Export{
					{Name: "foo", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "export empty and non-empty name",
			input: `(module
    (func)
    (func)
    (func)
    (export "" (func 2))
    (export "a" (func 1))
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0, 0, 0},
				CodeSection:     []*wasm.Code{{Body: end}, {Body: end}, {Body: end}},
				ExportSection: []*wasm.Export{
					{Name: "", Type: wasm.ExternTypeFunc, Index: wasm.Index(2)},
					{Name: "a", Type: wasm.ExternTypeFunc, Index: 1},
				},
			},
		},
		{
			name: "export memory - ID",
			input: `(module
    (memory $mem 1)
    (export "memory" (memory $mem))
)`,
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: wasm.MemoryLimitPages},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "start imported function by ID",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
				NameSection:  &wasm.NameSection{FunctionNames: wasm.NameMap{{Index: 0, Name: "hello"}}},
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{v_v},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
		},
		{
			name: "start function by ID",
			input: `(module
	(func $hello)
	(start $hello)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				StartSection:    &zero,
				NameSection:     &wasm.NameSection{FunctionNames: wasm.NameMap{{Index: 0, Name: "hello"}}},
			},
		},
		{
			name: "start function by ID - late",
			input: `(module
	(start $hello)
	(func $hello)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				StartSection:    &zero,
				NameSection:     &wasm.NameSection{FunctionNames: wasm.NameMap{{Index: 0, Name: "hello"}}},
			},
		},
		{
			name: "start function by index",
			input: `(module
	(func)
	(start 0)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				StartSection:    &zero,
			},
		},
		{
			name: "start function by index - late",
			input: `(module
	(start 0)
	(func)
)`,
			expected: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: end}},
				StartSection:    &zero,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := DecodeModule([]byte(tc.input), wasm.Features20220419, wasm.MemorySizer)
			require.NoError(t, err)
			require.Equal(t, tc.expected, m)
		})
	}
}

func TestParseModule_Errors(t *testing.T) {
	tests := []struct {
		name, input string
		expectedErr string
	}{
		{
			name:        "forgot parens",
			input:       "module",
			expectedErr: "1:1: expected '(', but parsed keyword: module",
		},
		{
			name:        "no module",
			input:       "()",
			expectedErr: "1:2: expected field, but parsed )",
		},
		{
			name:        "invalid",
			input:       "module",
			expectedErr: "1:1: expected '(', but parsed keyword: module",
		},
		{
			name:        "not module",
			input:       "(moodule)",
			expectedErr: "1:2: unexpected field: moodule",
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
			expectedErr: "1:15: expected field, but parsed ) in module",
		},
		{
			name:        "module trailing )",
			input:       "(module $foo ))",
			expectedErr: "1:15: found ')' before '('",
		},
		{
			name:        "module name after import",
			input:       "(module (import \"\" \"\" (func) $Math)",
			expectedErr: "1:30: unexpected ID: $Math in module.import[0]",
		},
		{
			name:        "type func ID clash",
			input:       "(module (type $1 (func)) (type $1 (func (param i32))))",
			expectedErr: "1:32: duplicate ID $1 in module.type[1]",
		},
		{
			name:        "type func multiple abbreviated results - multi-value disabled",
			input:       "(module (type (func (param i32 i32) (result i32 i32))))",
			expectedErr: "1:49: multiple result types invalid as feature \"multi-value\" is disabled in module.type[0].func.result[0]",
		},
		{
			name:        "type func multiple results - multi-value disabled",
			input:       "(module (type (func (param i32) (param i32) (result i32) (result i32))))",
			expectedErr: "1:59: multiple result types invalid as feature \"multi-value\" is disabled in module.type[0].func",
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
			name:        "import invalid token after name",
			input:       "(module (import \"foo\" \"bar\" $baz)",
			expectedErr: "1:29: unexpected ID: $baz in module.import[0]",
		},
		{
			name:        "import missing desc",
			input:       "(module (import \"foo\" \"bar\"))",
			expectedErr: "1:28: missing description field in module.import[0]",
		},
		{
			name:        "import empty desc",
			input:       "(module (import \"foo\" \"bar\"())",
			expectedErr: "1:29: expected field, but parsed ) in module.import[0]",
		},
		{
			name:        "import wrong end",
			input:       "(module (import \"foo\" \"bar\" (func) \"\"))",
			expectedErr: "1:36: unexpected string: \"\" in module.import[0]",
		},
		{
			name:        "import not desc field",
			input:       "(module (import \"foo\" \"bar\" ($func)))",
			expectedErr: "1:30: expected field, but parsed ID in module.import[0]",
		},
		{
			name:        "import wrong desc field",
			input:       "(module (import \"foo\" \"bar\" (funk)))",
			expectedErr: "1:30: unexpected field: funk in module.import[0]",
		},
		{
			name:        "import func invalid name",
			input:       "(module (import \"foo\" \"bar\" (func baz)))",
			expectedErr: "1:35: unexpected keyword: baz in module.import[0].func",
		},
		{
			name:        "import func invalid token",
			input:       "(module (import \"foo\" \"bar\" (func ($param))))",
			expectedErr: "1:36: unexpected ID: $param in module.import[0].func",
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
			name:        "import func multiple abbreviated results - multi-value disabled",
			input:       `(module (import "misc" "swap" (func $swap (param i32 i32) (result i32 i32))))`,
			expectedErr: "1:71: multiple result types invalid as feature \"multi-value\" is disabled in module.import[0].func.result[0]",
		},
		{
			name:        "import func multiple results - multi-value disabled",
			input:       `(module (import "misc" "swap" (func $swap (param i32) (param i32) (result i32) (result i32))))`,
			expectedErr: "1:81: multiple result types invalid as feature \"multi-value\" is disabled in module.import[0].func",
		},
		{
			name:        "import func wrong result type",
			input:       "(module (import \"\" \"\" (func (param i32) (result f65))))",
			expectedErr: "1:49: unknown type: f65 in module.import[0].func.result[0]",
		},
		{
			name:        "import func wrong param token",
			input:       "(module (import \"\" \"\" (func (param () ))))",
			expectedErr: "1:36: unexpected '(' in module.import[0].func.param[0]",
		},
		{
			name:        "import func wrong result token",
			input:       "(module (import \"\" \"\" (func (result () ))))",
			expectedErr: "1:37: unexpected '(' in module.import[0].func.result[0]",
		},
		{
			name:        "import func ID after param",
			input:       "(module (import \"\" \"\" (func (param i32) $main)))",
			expectedErr: "1:41: unexpected ID: $main in module.import[0].func",
		},
		{
			name:        "import func ID after result",
			input:       "(module (import \"\" \"\" (func (result i32) $main)))",
			expectedErr: "1:42: unexpected ID: $main in module.import[0].func",
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
			name:        "import func type mismatch",
			input:       `(module (type (func)) (import "" "" (func (type 0) (result i32)))`,
			expectedErr: "1:64: inlined type doesn't match module.type[0].func in module.import[0].func",
		},
		{
			name:        "import func type mismatch - late",
			input:       "(module (import \"\" \"\" (func (type 0) (result i32))) (type (func)))",
			expectedErr: "1:35: inlined type doesn't match module.type[0].func in module.import[0].func",
		},
		{
			name:        "import func type invalid",
			input:       "(module (import \"\" \"\" (func (type \"0\")))",
			expectedErr: "1:35: unexpected string: \"0\" in module.import[0].func.type",
		},
		{
			name:        "import func param after result",
			input:       "(module (import \"\" \"\" (func (result i32) (param i32))))",
			expectedErr: "1:43: param after result in module.import[0].func",
		},
		{
			name:        "import func double desc",
			input:       "(module (import \"foo\" \"bar\" (func $main) (func $mein)))",
			expectedErr: "1:42: unexpected '(' in module.import[0]",
		},
		{
			name:        "import func wrong end",
			input:       "(module (import \"foo\" \"bar\" (func \"\")))",
			expectedErr: "1:35: unexpected string: \"\" in module.import[0].func",
		},
		{
			name:        "import func points nowhere",
			input:       "(module (import \"foo\" \"bar\" (func (type $v_v))))",
			expectedErr: "1:41: unknown ID $v_v",
		},
		{
			name:        "import func after func",
			input:       "(module (func) (import \"\" \"\" (func)))",
			expectedErr: "1:31: import after module-defined function in module.import[0]",
		},
		{
			name:        "func invalid name",
			input:       "(module (func baz)))",
			expectedErr: "1:15: unsupported instruction: baz in module.func[0]",
		},
		{
			name:        "func invalid token",
			input:       "(module (func ($param)))",
			expectedErr: "1:16: unexpected ID: $param in module.func[0]",
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
			name:        "func multiple abbreviated results - multi-value disabled",
			input:       "(module (func $swap (param i32 i32) (result i32 i32) local.get 1 local.get 0))",
			expectedErr: "1:49: multiple result types invalid as feature \"multi-value\" is disabled in module.func[0].result[0]",
		},
		{
			name:        "func multiple results - multi-value disabled",
			input:       "(module (func $swap (param i32) (param i32) (result i32) (result i32) local.get 1 local.get 0))",
			expectedErr: "1:59: multiple result types invalid as feature \"multi-value\" is disabled in module.func[0]",
		},
		{
			name:        "func wrong result type",
			input:       "(module (func (param i32) (result f65)))",
			expectedErr: "1:35: unknown type: f65 in module.func[0].result[0]",
		},
		{
			name:        "func wrong param token",
			input:       "(module (func (param () )))",
			expectedErr: "1:22: unexpected '(' in module.func[0].param[0]",
		},
		{
			name:        "func wrong result token",
			input:       "(module (func (result () )))",
			expectedErr: "1:23: unexpected '(' in module.func[0].result[0]",
		},
		{
			name:        "func ID after param",
			input:       "(module (func (param i32) $main))",
			expectedErr: "1:27: unexpected ID: $main in module.func[0]",
		},
		{
			name:        "func wrong end",
			input:       "(module (func $main \"\"))",
			expectedErr: "1:21: unexpected string: \"\" in module.func[0]",
		},
		{
			name:        "clash on func ID",
			input:       "(module (func $main) (func $main)))",
			expectedErr: "1:28: duplicate ID $main in module.func[1]",
		},
		{
			name:        "func ID clashes with import func ID",
			input:       "(module (import \"\" \"\" (func $main)) (func $main)))",
			expectedErr: "1:43: duplicate ID $main in module.func[0]",
		},
		{
			name:        "func ID after result",
			input:       "(module (func (result i32) $main)))",
			expectedErr: "1:28: unexpected ID: $main in module.func[0]",
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
			name:        "func type mismatch",
			input:       "(module (type (func)) (func (type 0) (result i32))",
			expectedErr: "1:50: inlined type doesn't match module.type[0].func in module.func[0]",
		},
		{
			name:        "func type mismatch - late",
			input:       "(module (func (type 0) (result i32)) (type (func)))",
			expectedErr: "1:21: inlined type doesn't match module.type[0].func in module.func[0]",
		},
		{
			name:        "func type invalid",
			input:       "(module (func (type \"0\")))",
			expectedErr: "1:21: unexpected string: \"0\" in module.func[0].type",
		},
		{
			name:        "func param after result",
			input:       "(module (func (result i32) (param i32)))",
			expectedErr: "1:29: param after result in module.func[0]",
		},
		{
			name:        "func points nowhere",
			input:       "(module (func (type $v_v)))",
			expectedErr: "1:21: unknown ID $v_v",
		},
		{
			name: "func call unresolved - index",
			input: `(module
			(func)
			(func call 2)
		)`,
			expectedErr: "3:15: index 2 is out of range [0..1] in module.code[1].body[1]",
		},
		{
			name: "func call unresolved - ID",
			input: `(module
			(func $main)
			(func call $mein)
		)`,
			expectedErr: "3:15: unknown ID $mein in module.code[1].body[1]",
		},
		{
			name: "func sign-extension disabled",
			input: `(module
			(func (param i64) (result i64) local.get 0 i64.extend16_s)
		)`,
			expectedErr: "2:47: i64.extend16_s invalid as feature \"sign-extension-ops\" is disabled in module.func[0]",
		},
		{
			name: "func nontrapping-float-to-int-conversions disabled",
			input: `(module
			(func (param f32) (result i32) local.get 0 i32.trunc_sat_f32_s)
		)`,
			expectedErr: "2:47: i32.trunc_sat_f32_s invalid as feature \"nontrapping-float-to-int-conversion\" is disabled in module.func[0]",
		},
		{
			name:        "memory over max",
			input:       "(module (memory 1 70000))",
			expectedErr: "1:19: max 70000 pages (4 Gi) over limit of 65536 pages (4 Gi) in module.memory[0]",
		},
		{
			name:        "second memory",
			input:       "(module (memory 1) (memory 1))",
			expectedErr: "1:21: at most one memory allowed in module",
		},
		{
			name: "export duplicates empty name",
			input: `(module
    (func)
	(func)
    (export "" (func 0))
    (export "" (memory 1))
)`,
			expectedErr: `5:13: "" already exported in module.export[1]`,
		},
		{
			name: "export duplicates name",
			input: `(module
    (func)
	(func)
    (export "a" (func 0))
    (export "a" (memory 1))
)`,
			expectedErr: `5:13: "a" already exported in module.export[1]`,
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
			name:        "export wrong end",
			input:       "(module (export \"PI\" (func $main) \"\"))",
			expectedErr: "1:35: unexpected string: \"\" in module.export[0]",
		},
		{
			name:        "export missing name",
			input:       "(module (export (func)))",
			expectedErr: "1:17: missing name in module.export[0]",
		},
		{
			name:        "export missing desc",
			input:       "(module (export \"foo\")))",
			expectedErr: "1:22: missing description field in module.export[0]",
		},
		{
			name:        "export not desc",
			input:       "(module (export \"foo\" $func))",
			expectedErr: "1:23: unexpected ID: $func in module.export[0]",
		},
		{
			name:        "export not desc field",
			input:       "(module (export \"foo\" ($func)))",
			expectedErr: "1:24: expected field, but parsed ID in module.export[0]",
		},
		{
			name:        "export wrong desc field",
			input:       "(module (export \"foo\" (funk)))",
			expectedErr: "1:24: unexpected field: funk in module.export[0]",
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
			name:        "export func wrong end",
			input:       "(module (export \"PI\" (func $main \"\")))",
			expectedErr: "1:34: unexpected string: \"\" in module.export[0].func",
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
			name:        "start wrong end",
			input:       "(module (start $main \"\"))",
			expectedErr: "1:22: unexpected string: \"\" in module.start",
		},
		{
			name:        "double start",
			input:       "(module (start $main) (start $main))",
			expectedErr: "1:24: at most one start allowed in module",
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
			_, err := DecodeModule([]byte(tc.input), wasm.Features20191205, wasm.MemorySizer)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModuleParser_ErrorContext(t *testing.T) {
	p := newModuleParser(&wasm.Module{}, 0, wasm.MemorySizer)
	tests := []struct {
		input    string
		pos      parserPosition
		expected string
	}{
		{input: "initial", pos: positionInitial, expected: ""},
		{input: "module", pos: positionModule, expected: "module"},
		{input: "module import", pos: positionImport, expected: "module.import[0]"},
		{input: "module import func", pos: positionImportFunc, expected: "module.import[0].func"},
		{input: "module func", pos: positionFunc, expected: "module.func[0]"},
		{input: "module memory", pos: positionMemory, expected: "module.memory[0]"},
		{input: "module export", pos: positionExport, expected: "module.export[0]"},
		{input: "module export func", pos: positionExportFunc, expected: "module.export[0].func"},
		{input: "start", pos: positionStart, expected: "module.start"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.input, func(t *testing.T) {
			p.pos = tc.pos
			require.Equal(t, tc.expected, p.errorContext())
		})
	}
}
