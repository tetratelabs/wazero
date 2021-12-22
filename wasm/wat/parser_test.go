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
			m, line, col, err := ParseModule([]byte(tc.input))
			require.NoError(t, err, "%d:%d: %s", line, col, err)
			require.Equal(t, tc.expected, m)
		})
	}
}

func TestParseModule_Errors(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		expectedLine int
		expectedCol  int
		expectedErr  string
	}{
		{
			name:         "no module",
			input:        []byte("()"),
			expectedLine: 1,
			expectedCol:  2,
			expectedErr:  "module has a ) where a field name was expected",
		},
		{
			name:         "module invalid name",
			input:        []byte("(module test)"), // must start with $
			expectedLine: 1,
			expectedCol:  9,
			expectedErr:  "module has an unexpected keyword: test",
		},
		{
			name:         "module double name",
			input:        []byte("(module $foo $bar)"),
			expectedLine: 1,
			expectedCol:  14,
			expectedErr:  "module has a redundant name: $bar",
		},
		{
			name:         "module empty field",
			input:        []byte("(module $foo ())"),
			expectedLine: 1,
			expectedCol:  15,
			expectedErr:  "module has a ) where a field name was expected",
		},
		{
			name:         "module trailing )",
			input:        []byte("(module $foo ))"),
			expectedLine: 1,
			expectedCol:  15,
			expectedErr:  "found ')' before '('",
		},
		{
			name:         "import missing module",
			input:        []byte("(module (import))"),
			expectedLine: 1,
			expectedCol:  16,
			expectedErr:  "import[1] is missing its module and name",
		},
		{
			name:         "import missing name",
			input:        []byte("(module (import \"\"))"),
			expectedLine: 1,
			expectedCol:  19,
			expectedErr:  "import[1] is missing its name",
		},
		{
			name:         "import unquoted module",
			input:        []byte("(module (import foo bar))"),
			expectedLine: 1,
			expectedCol:  17,
			expectedErr:  "import[1] has an unexpected keyword: foo",
		},
		{
			name:         "import double name",
			input:        []byte("(module (import \"foo\" \"bar\" \"baz\")"),
			expectedLine: 1,
			expectedCol:  29,
			expectedErr:  "import[1] has a redundant name: baz",
		},
		{
			name:         "import missing desc",
			input:        []byte("(module (import \"foo\" \"bar\"))"),
			expectedLine: 1,
			expectedCol:  28,
			expectedErr:  "import[1] is missing its descripton",
		},
		{
			name:         "import desc empty",
			input:        []byte("(module (import \"foo\" \"bar\"())"),
			expectedLine: 1,
			expectedCol:  29,
			expectedErr:  "import[1] has a ) where a field name was expected",
		},
		{
			name:         "import func invalid name",
			input:        []byte("(module (import \"foo\" \"bar\" (func baz)))"),
			expectedLine: 1,
			expectedCol:  35,
			expectedErr:  "import[1].func has an unexpected keyword: baz",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, line, col, err := ParseModule(tc.input)
			require.Equal(t, tc.expectedLine, line)
			require.Equal(t, tc.expectedCol, col)
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
			line, col, err := lex(noopParseToken, simpleExample)
			if err != nil {
				panic(fmt.Errorf("%d:%d: %w", line, col, err))
			}
		}
	})
	b.Run("ParseModule", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, line, col, err := ParseModule(simpleExample)
			if err != nil {
				panic(fmt.Errorf("%d:%d: %w", line, col, err))
			}
		}
	})
}
