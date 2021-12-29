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
		expected *textModule
	}{
		{
			name:     "empty",
			input:    "(module)",
			expected: &textModule{},
		},
		{
			name:     "only name",
			input:    "(module $tools)",
			expected: &textModule{name: "$tools"},
		},
		{
			name:     "import func empty",
			input:    "(module (import \"foo\" \"bar\" (func)))", // ok empty sig
			expected: &textModule{imports: []*textImport{{module: "foo", name: "bar", desc: &textFunc{}}}},
		},
		{
			name: "start imported function by name",
			input: `(module
	(import "" "hello" (func $hello))
	(start $hello)
)`,
			expected: &textModule{
				imports:       []*textImport{{name: "hello", desc: &textFunc{name: "$hello"}}},
				startFunction: "$hello",
			},
		},
		{
			name: "start imported function by index",
			input: `(module
	(import "" "hello" (func))
	(start 0)
)`,
			expected: &textModule{
				imports:       []*textImport{{name: "hello", desc: &textFunc{}}},
				startFunction: "0",
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseModule([]byte(tc.input))
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
			expectedErr: "1:2: expected field, but found ) in module",
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
			expectedErr: "1:15: found ')' before '(' in module",
		},
		{
			name:        "import missing module",
			input:       []byte("(module (import))"),
			expectedErr: "1:16: expected module and name in import[0]",
		},
		{
			name:        "import missing name",
			input:       []byte("(module (import \"\"))"),
			expectedErr: "1:19: expected name in import[0]",
		},
		{
			name:        "import unquoted module",
			input:       []byte("(module (import foo bar))"),
			expectedErr: "1:17: unexpected keyword: foo in import[0]",
		},
		{
			name:        "import double name",
			input:       []byte("(module (import \"foo\" \"bar\" \"baz\")"),
			expectedErr: "1:29: redundant name: baz in import[0]",
		},
		{
			name:        "import missing desc",
			input:       []byte("(module (import \"foo\" \"bar\"))"),
			expectedErr: "1:28: expected descripton in import[0]",
		},
		{
			name:        "import desc empty",
			input:       []byte("(module (import \"foo\" \"bar\"())"),
			expectedErr: "1:29: expected field, but found ) in import[0]",
		},
		{
			name:        "import func invalid name",
			input:       []byte("(module (import \"foo\" \"bar\" (func baz)))"),
			expectedErr: "1:35: unexpected keyword: baz in import[0].func",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModule(tc.input)
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
	b.Run("ParseModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseModule(simpleExample)
			if err != nil {
				panic(err)
			}
		}
	})
}
