package wat

import (
	"fmt"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestTextToBinary(t *testing.T) {
	zero := uint32(0)
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
			name:  "import func two",
			input: "(module (import \"foo\" \"bar\" (func)) (import \"baz\" \"qux\" (func)))",
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.ImportSegment{
					{
						Module: "foo", Name: "bar",
						Desc: &wasm.ImportDesc{
							Kind:          wasm.ImportKindFunction,
							FuncTypeIndex: 0,
						},
					}, {
						Module: "baz", Name: "qux",
						Desc: &wasm.ImportDesc{
							Kind:          wasm.ImportKindFunction,
							FuncTypeIndex: 0,
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
			name:        "start, but no funcs",
			input:       "(module (start $main))",
			expectedErr: "1:16: unknown function name: $main in module.start",
		},
		{
			name: "start index missing - number",
			input: `(module
	(import "" "hello" (func))
	(start 1)
)`,
			expectedErr: "3:9: invalid function index: 1 in module.start",
		},
		{
			name: "start index missing - name",
			input: `(module
	(import "" "hello" (func $main))
	(start $mein)
)`,
			expectedErr: "3:9: unknown function name: $mein in module.start",
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

func BenchmarkTextToBinaryExample(b *testing.B) {
	simpleExample := []byte(`(module
	(import "" "hello" (func $hello))
	(start $hello)
)`)

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
		bin, err := os.ReadFile("testdata/simple.wasm")
		if err != nil {
			panic(err)
		}
		for i := 0; i < b.N; i++ {
			_, err = wasm.DecodeModule(bin)
			if err != nil {
				panic(err)
			}
		}
	})
	b.Run("vs wat.lex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			line, col, err := lex(noopTokenParser, simpleExample)
			if err != nil {
				panic(fmt.Errorf("%d:%d: %w", line, col, err))
			}
		}
	})
	b.Run("vs wat.parseModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := parseModule(simpleExample)
			if err != nil {
				panic(err)
			}
		}
	})
	b.Run("TextToBinary", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := TextToBinary(simpleExample)
			if err != nil {
				panic(err)
			}
		}
	})
}
