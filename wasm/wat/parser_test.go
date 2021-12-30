package wat

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
)

func TestParseModule(t *testing.T) {
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
			name:     "import func empty",
			input:    "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &module{imports: []*_import{{module: "foo", name: "bar", importFunc: &importFunc{}}}},
		},
		{
			name:     "start function", // TODO: this is pointing to a funcidx not in the source!
			input:    "(module (start $main))",
			expected: &module{startFunction: "$main"},
		},
		{
			name: "start imported function by name",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &module{
				imports:       []*_import{{name: "hello", importFunc: &importFunc{name: "$hello"}}},
				startFunction: "$hello",
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &module{
				imports:       []*_import{{name: "hello", importFunc: &importFunc{}}},
				startFunction: "0",
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
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "no module",
			input:       []byte("()"),
			expectedErr: "1:2: expected field, but found )",
		},
		{
			name:        "module invalid name",
			input:       []byte("(module test)"), // must start with $
			expectedErr: "1:9: unexpected keyword: test in module",
		},
		{
			name:        "module double name",
			input:       []byte("(module $foo $bar)"),
			expectedErr: "1:14: redundant name: $bar in module",
		},
		{
			name:        "module empty field",
			input:       []byte("(module $foo ())"),
			expectedErr: "1:15: expected field, but found ) in module",
		},
		{
			name:        "module trailing )",
			input:       []byte("(module $foo ))"),
			expectedErr: "1:15: found ')' before '('",
		},
		{
			name:        "import missing module",
			input:       []byte("(module (import))"),
			expectedErr: "1:16: expected module and name in module.import[0]",
		},
		{
			name:        "import missing name",
			input:       []byte("(module (import \"\"))"),
			expectedErr: "1:19: expected name in module.import[0]",
		},
		{
			name:        "import unquoted module",
			input:       []byte("(module (import foo bar))"),
			expectedErr: "1:17: unexpected keyword: foo in module.import[0]",
		},
		{
			name:        "import double name",
			input:       []byte("(module (import \"foo\" \"bar\" \"baz\")"),
			expectedErr: "1:29: redundant name: baz in module.import[0]",
		},
		{
			name:        "import missing importFunc",
			input:       []byte("(module (import \"foo\" \"bar\"))"),
			expectedErr: "1:28: expected description in module.import[0]",
		},
		{
			name:        "import importFunc empty",
			input:       []byte("(module (import \"foo\" \"bar\"())"),
			expectedErr: "1:29: expected field, but found ) in module.import[0]",
		},
		{
			name:        "import func invalid name",
			input:       []byte("(module (import \"foo\" \"bar\" (func baz)))"),
			expectedErr: "1:35: unexpected keyword: baz in module.import[0].func",
		},
		{
			name:        "start missing funcidx",
			input:       []byte("(module (start))"),
			expectedErr: "1:15: missing funcidx in module.start",
		},
		{
			name:        "start double funcidx",
			input:       []byte("(module (start $main $main))"),
			expectedErr: "1:22: redundant funcidx: $main in module.start",
		},
		{
			name:        "double start",
			input:       []byte("(module (start $main) (start $main))"),
			expectedErr: "1:30: redundant funcidx: $main in module.start",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := parseModule(tc.input)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func BenchmarkParseExample(b *testing.B) {
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
	// Not a fair comparison as we are only parsing and not writing back %.wasm
	// If possible, we should find a way to isolate only the lexing C Functions.
	b.Run("vs wasmtime.Wat2Wasm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := wasmtime.Wat2Wasm(string(simpleExample))
			if err != nil {
				panic(err)
			}
		}
	})
	b.Run("vs wat.lex(noop)", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			line, col, err := lex(noopTokenParser, simpleExample)
			if err != nil {
				panic(fmt.Errorf("%d:%d: %w", line, col, err))
			}
		}
	})
	b.Run("parseModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := parseModule(simpleExample)
			if err != nil {
				panic(err)
			}
		}
	})
}
