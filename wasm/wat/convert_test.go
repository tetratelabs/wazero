package wat

import (
	"os"
	"testing"
	"unicode/utf8"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestTextToBinary(t *testing.T) {
	zero := uint32(0)
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64
	tests := []struct {
		name     string
		input    string
		expected *wasm.Module
	}{
		{
			name:     "empty",
			input:    "(module)",
			expected: &wasm.Module{},
		},
		{
			name:  "import func empty",
			input: "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.ImportSegment{{
					Module: "foo", Name: "bar",
					Desc: &wasm.ImportDesc{
						Kind:          wasm.ImportKindFunction,
						FuncTypeIndex: 0,
					},
				}},
			},
		},
		{
			name: "multiple import func with different inlined type",
			input: `(module
	(import "wasi_snapshot_preview1" "path_open" (func $runtime.path_open (param i32 i32 i32 i32 i32 i64 i64 i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write (param i32 i32 i32 i32) (result i32)))
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				ImportSection: []*wasm.ImportSegment{
					{
						Module: "wasi_snapshot_preview1", Name: "path_open",
						Desc: &wasm.ImportDesc{
							Kind:          wasm.ImportKindFunction,
							FuncTypeIndex: 0,
						},
					}, {
						Module: "wasi_snapshot_preview1", Name: "fd_write",
						Desc: &wasm.ImportDesc{
							Kind:          wasm.ImportKindFunction,
							FuncTypeIndex: 1,
						},
					},
				},
			},
		},
		{
			name: "start imported function by name",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.ImportSegment{{
					Module: "", Name: "hello",
					Desc: &wasm.ImportDesc{
						Kind:          wasm.ImportKindFunction,
						FuncTypeIndex: 0,
					},
				}},
				StartSection: &zero,
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.ImportSegment{{
					Module: "", Name: "hello",
					Desc: &wasm.ImportDesc{
						Kind:          wasm.ImportKindFunction,
						FuncTypeIndex: 0,
					},
				}},
				StartSection: &zero,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := TextToBinary([]byte(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, m)
		})
	}
}

func TestTextToBinary_Errors(t *testing.T) {
	tests := []struct{ name, input, expectedErr string }{
		{
			name:        "invalid",
			input:       "module",
			expectedErr: "1:1: expected '(', but found keyword: module",
		},
		{
			name:        "start, but no funcs",
			input:       "(module (start $main))",
			expectedErr: "1:16: unknown function name $main in module.start",
		},
		{
			name: "start index out of range",
			input: `(module
	(import "" "hello" (func))
	(import "" "goodbye" (func))
	(start 3)
)`,
			expectedErr: "4:9: function index 3 is out of range [0..1] in module.start",
		},
		{
			name: "start points to unknown func",
			input: `(module
	(import "" "hello" (func $main))
	(start $mein)
)`,
			expectedErr: "3:9: unknown function name $mein in module.start",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := TextToBinary([]byte(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

var simpleExample = []byte(`(module $simple
	(import "" "hello" (func $hello))
	(start $hello)
)`)

func BenchmarkTextToBinaryExample(b *testing.B) {
	var simpleExampleBinary []byte
	if bin, err := os.ReadFile("testdata/simple.wasm"); err != nil {
		b.Fatal(err)
	} else {
		simpleExampleBinary = bin
	}

	b.Run("vs utf8.Valid", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !utf8.Valid(simpleExample) {
				panic("unexpected")
			}
		}
	})
	// Not a fair comparison as while TextToBinary parses into the binary format, we don't encode it into a byte slice.
	// We also don't know if wasmtime.Wat2Wasm encodes the custom name section or not.
	b.Run("vs wasmtime.Wat2Wasm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := wasmtime.Wat2Wasm(string(simpleExample))
			if err != nil {
				panic(err)
			}
		}
	})
	// This compares against reading the same binary data directly (encoded via wat2wasm --debug-names).
	// Note: This will be more similar once TextToBinary writes CustomSection["name"]
	b.Run("vs wasm.DecodeModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := wasm.DecodeModule(simpleExampleBinary); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("vs wat.lex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if line, col, err := lex(noopTokenParser, simpleExample); err != nil {
				b.Fatalf("%d:%d: %s", line, col, err)
			}
		}
	})
	b.Run("vs wat.parseModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parseModule(simpleExample); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("TextToBinary", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := TextToBinary(simpleExample); err != nil {
				b.Fatal(err)
			}
		}
	})
}
