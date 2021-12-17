package wat

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
)

// exampleWat was at one time in the wasmtime repo under cranelift. We added a unicode comment for fun!
const exampleWat = `(module
  ;; 私たちはフィボナッチ数列を使います。何故ならみんなやってるからです。
  (memory 1)
  (func $main (local i32 i32 i32 i32)
    (set_local 0 (i32.const 0))
    (set_local 1 (i32.const 1))
    (set_local 2 (i32.const 1))
    (set_local 3 (i32.const 0))
    (block
    (loop
        (br_if 1 (i32.gt_s (get_local 0) (i32.const 5)))
        (set_local 3 (get_local 2))
        (set_local 2 (i32.add (get_local 2) (get_local 1)))
        (set_local 1 (get_local 3))
        (set_local 0 (i32.add (get_local 0) (i32.const 1)))
        (br 0)
    )
    )
    (i32.store (i32.const 0) (get_local 2))
  )
  (start $main)
  (data (i32.const 0) "0000")
)`

// TestLex_Example is intentionally verbose to catch line/column/position bugs
func TestLex_Example(t *testing.T) {
	tokens, e := lexTokens(exampleWat)
	require.NoError(t, e)
	require.Equal(t, []*token{
		{tokenLParen, 1, 1, 0, "("},
		{tokenKeyword, 1, 2, 1, "module"},
		{tokenLParen, 3, 3, 118, "("},
		{tokenKeyword, 3, 4, 119, "memory"},
		{tokenUN, 3, 11, 126, "1"},
		{tokenRParen, 3, 12, 127, ")"},
		{tokenLParen, 4, 3, 131, "("},
		{tokenKeyword, 4, 4, 132, "func"},
		{tokenID, 4, 9, 137, "$main"},
		{tokenLParen, 4, 15, 143, "("},
		{tokenKeyword, 4, 16, 144, "local"},
		{tokenKeyword, 4, 22, 150, "i32"},
		{tokenKeyword, 4, 26, 154, "i32"},
		{tokenKeyword, 4, 30, 158, "i32"},
		{tokenKeyword, 4, 34, 162, "i32"},
		{tokenRParen, 4, 37, 165, ")"},
		{tokenLParen, 5, 5, 171, "("},
		{tokenKeyword, 5, 6, 172, "set_local"},
		{tokenUN, 5, 16, 182, "0"},
		{tokenLParen, 5, 18, 184, "("},
		{tokenKeyword, 5, 19, 185, "i32.const"},
		{tokenUN, 5, 29, 195, "0"},
		{tokenRParen, 5, 30, 196, ")"},
		{tokenRParen, 5, 31, 197, ")"},
		{tokenLParen, 6, 5, 203, "("},
		{tokenKeyword, 6, 6, 204, "set_local"},
		{tokenUN, 6, 16, 214, "1"},
		{tokenLParen, 6, 18, 216, "("},
		{tokenKeyword, 6, 19, 217, "i32.const"},
		{tokenUN, 6, 29, 227, "1"},
		{tokenRParen, 6, 30, 228, ")"},
		{tokenRParen, 6, 31, 229, ")"},
		{tokenLParen, 7, 5, 235, "("},
		{tokenKeyword, 7, 6, 236, "set_local"},
		{tokenUN, 7, 16, 246, "2"},
		{tokenLParen, 7, 18, 248, "("},
		{tokenKeyword, 7, 19, 249, "i32.const"},
		{tokenUN, 7, 29, 259, "1"},
		{tokenRParen, 7, 30, 260, ")"},
		{tokenRParen, 7, 31, 261, ")"},
		{tokenLParen, 8, 5, 267, "("},
		{tokenKeyword, 8, 6, 268, "set_local"},
		{tokenUN, 8, 16, 278, "3"},
		{tokenLParen, 8, 18, 280, "("},
		{tokenKeyword, 8, 19, 281, "i32.const"},
		{tokenUN, 8, 29, 291, "0"},
		{tokenRParen, 8, 30, 292, ")"},
		{tokenRParen, 8, 31, 293, ")"},
		{tokenLParen, 9, 5, 299, "("},
		{tokenKeyword, 9, 6, 300, "block"},
		{tokenLParen, 10, 5, 310, "("},
		{tokenKeyword, 10, 6, 311, "loop"},
		{tokenLParen, 11, 9, 324, "("},
		{tokenKeyword, 11, 10, 325, "br_if"},
		{tokenUN, 11, 16, 331, "1"},
		{tokenLParen, 11, 18, 333, "("},
		{tokenKeyword, 11, 19, 334, "i32.gt_s"},
		{tokenLParen, 11, 28, 343, "("},
		{tokenKeyword, 11, 29, 344, "get_local"},
		{tokenUN, 11, 39, 354, "0"},
		{tokenRParen, 11, 40, 355, ")"},
		{tokenLParen, 11, 42, 357, "("},
		{tokenKeyword, 11, 43, 358, "i32.const"},
		{tokenUN, 11, 53, 368, "5"},
		{tokenRParen, 11, 54, 369, ")"},
		{tokenRParen, 11, 55, 370, ")"},
		{tokenRParen, 11, 56, 371, ")"},
		{tokenLParen, 12, 9, 381, "("},
		{tokenKeyword, 12, 10, 382, "set_local"},
		{tokenUN, 12, 20, 392, "3"},
		{tokenLParen, 12, 22, 394, "("},
		{tokenKeyword, 12, 23, 395, "get_local"},
		{tokenUN, 12, 33, 405, "2"},
		{tokenRParen, 12, 34, 406, ")"},
		{tokenRParen, 12, 35, 407, ")"},
		{tokenLParen, 13, 9, 417, "("},
		{tokenKeyword, 13, 10, 418, "set_local"},
		{tokenUN, 13, 20, 428, "2"},
		{tokenLParen, 13, 22, 430, "("},
		{tokenKeyword, 13, 23, 431, "i32.add"},
		{tokenLParen, 13, 31, 439, "("},
		{tokenKeyword, 13, 32, 440, "get_local"},
		{tokenUN, 13, 42, 450, "2"},
		{tokenRParen, 13, 43, 451, ")"},
		{tokenLParen, 13, 45, 453, "("},
		{tokenKeyword, 13, 46, 454, "get_local"},
		{tokenUN, 13, 56, 464, "1"},
		{tokenRParen, 13, 57, 465, ")"},
		{tokenRParen, 13, 58, 466, ")"},
		{tokenRParen, 13, 59, 467, ")"},
		{tokenLParen, 14, 9, 477, "("},
		{tokenKeyword, 14, 10, 478, "set_local"},
		{tokenUN, 14, 20, 488, "1"},
		{tokenLParen, 14, 22, 490, "("},
		{tokenKeyword, 14, 23, 491, "get_local"},
		{tokenUN, 14, 33, 501, "3"},
		{tokenRParen, 14, 34, 502, ")"},
		{tokenRParen, 14, 35, 503, ")"},
		{tokenLParen, 15, 9, 513, "("},
		{tokenKeyword, 15, 10, 514, "set_local"},
		{tokenUN, 15, 20, 524, "0"},
		{tokenLParen, 15, 22, 526, "("},
		{tokenKeyword, 15, 23, 527, "i32.add"},
		{tokenLParen, 15, 31, 535, "("},
		{tokenKeyword, 15, 32, 536, "get_local"},
		{tokenUN, 15, 42, 546, "0"},
		{tokenRParen, 15, 43, 547, ")"},
		{tokenLParen, 15, 45, 549, "("},
		{tokenKeyword, 15, 46, 550, "i32.const"},
		{tokenUN, 15, 56, 560, "1"},
		{tokenRParen, 15, 57, 561, ")"},
		{tokenRParen, 15, 58, 562, ")"},
		{tokenRParen, 15, 59, 563, ")"},
		{tokenLParen, 16, 9, 573, "("},
		{tokenKeyword, 16, 10, 574, "br"},
		{tokenUN, 16, 13, 577, "0"},
		{tokenRParen, 16, 14, 578, ")"},
		{tokenRParen, 17, 5, 584, ")"},
		{tokenRParen, 18, 5, 590, ")"},
		{tokenLParen, 19, 5, 596, "("},
		{tokenKeyword, 19, 6, 597, "i32.store"},
		{tokenLParen, 19, 16, 607, "("},
		{tokenKeyword, 19, 17, 608, "i32.const"},
		{tokenUN, 19, 27, 618, "0"},
		{tokenRParen, 19, 28, 619, ")"},
		{tokenLParen, 19, 30, 621, "("},
		{tokenKeyword, 19, 31, 622, "get_local"},
		{tokenUN, 19, 41, 632, "2"},
		{tokenRParen, 19, 42, 633, ")"},
		{tokenRParen, 19, 43, 634, ")"},
		{tokenRParen, 20, 3, 638, ")"},
		{tokenLParen, 21, 3, 642, "("},
		{tokenKeyword, 21, 4, 643, "start"},
		{tokenID, 21, 10, 649, "$main"},
		{tokenRParen, 21, 15, 654, ")"},
		{tokenLParen, 22, 3, 658, "("},
		{tokenKeyword, 22, 4, 659, "data"},
		{tokenLParen, 22, 9, 664, "("},
		{tokenKeyword, 22, 10, 665, "i32.const"},
		{tokenUN, 22, 20, 675, "0"},
		{tokenRParen, 22, 21, 676, ")"},
		{tokenString, 22, 23, 678, "\"0000\""},
		{tokenRParen, 22, 29, 684, ")"},
		{tokenRParen, 23, 1, 686, ")"},
	}, tokens)
}

func TestLex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []*token
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:     "only parens",
			input:    "()",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenRParen, 1, 2, 1, ")"}},
		},
		{
			name:     "shortest keywords",
			input:    "a z",
			expected: []*token{{tokenKeyword, 1, 1, 0, "a"}, {tokenKeyword, 1, 3, 2, "z"}},
		},
		{
			name:     "shortest tokens - EOL",
			input:    "(a)\n",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenKeyword, 1, 2, 1, "a"}, {tokenRParen, 1, 3, 2, ")"}},
		},
		{
			name:     "only tokens",
			input:    "(module)",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenKeyword, 1, 2, 1, "module"}, {tokenRParen, 1, 8, 7, ")"}},
		},
		{
			name:  "only white space characters",
			input: " \t\r\n",
		},
		{
			name:     "after white space characters - EOL",
			input:    " \t\na",
			expected: []*token{{tokenKeyword, 2, 1, 3, "a"}},
		},
		{
			name:     "after white space characters - Windows EOL",
			input:    " \t\r\na",
			expected: []*token{{tokenKeyword, 2, 1, 4, "a"}},
		},
		{
			name:  "only line comment - EOL before EOF",
			input: ";; TODO\n",
		},
		{
			name:  "only line comment - EOF",
			input: ";; TODO",
		},
		{
			name:  "only unicode line comment - EOF",
			input: ";; брэд-ЛГТМ",
		},
		{
			name:     "after line comment",
			input:    ";; TODO\na",
			expected: []*token{{tokenKeyword, 2, 1, 8, "a"}},
		},
		{
			name:     "double line comment",
			input:    ";; TODO\n;; YOLO\na",
			expected: []*token{{tokenKeyword, 3, 1, 16, "a"}},
		},
		{
			name:     "after unicode line comment",
			input:    ";; брэд-ЛГТМ\na",
			expected: []*token{{tokenKeyword, 2, 1, 21, "a"}},
		},
		{
			name:     "after line comment - Windows EOL",
			input:    ";; TODO\r\na",
			expected: []*token{{tokenKeyword, 2, 1, 9, "a"}},
		},
		{
			name:     "after redundant line comment",
			input:    ";;;; TODO\na",
			expected: []*token{{tokenKeyword, 2, 1, 10, "a"}},
		},
		{
			name:     "after line commenting out block comment",
			input:    ";; TODO (; ;)\na",
			expected: []*token{{tokenKeyword, 2, 1, 14, "a"}},
		},
		{
			name:     "after line commenting out open block comment",
			input:    ";; TODO (;\na",
			expected: []*token{{tokenKeyword, 2, 1, 11, "a"}},
		},
		{
			name:     "after line commenting out close block comment",
			input:    ";; TODO ;)\na",
			expected: []*token{{tokenKeyword, 2, 1, 11, "a"}},
		},
		{
			name:  "only block comment - EOL before EOF",
			input: "(; TODO ;)\n",
		},
		{
			name:  "only block comment - Windows EOL before EOF",
			input: "(; TODO ;)\r\n",
		},
		{
			name:  "only block comment - EOF",
			input: "(; TODO ;)",
		},
		{
			name:     "double block comment",
			input:    "(; TODO ;)(; YOLO ;)a",
			expected: []*token{{tokenKeyword, 1, 21, 20, "a"}},
		},
		{
			name:     "double block comment - EOL",
			input:    "(; TODO ;)\n(; YOLO ;)\na",
			expected: []*token{{tokenKeyword, 3, 1, 22, "a"}},
		},
		{
			name:     "after block comment",
			input:    "(; TODO ;)a",
			expected: []*token{{tokenKeyword, 1, 11, 10, "a"}},
		},
		{
			name:  "only nested block comment - EOL before EOF",
			input: "(; TODO (; (YOLO) ;) ;)\n",
		},
		{
			name:  "only nested block comment - EOF",
			input: "(; TODO (; (YOLO) ;) ;)",
		},
		{
			name:  "only unicode block comment - EOF",
			input: "(; брэд-ЛГТМ ;)",
		},
		{
			name:     "after nested block comment",
			input:    "(; TODO (; (YOLO) ;) ;)a",
			expected: []*token{{tokenKeyword, 1, 24, 23, "a"}},
		},
		{
			name:     "after nested block comment - EOL",
			input:    "(; TODO (; (YOLO) ;) ;)\n a",
			expected: []*token{{tokenKeyword, 2, 2, 25, "a"}},
		},
		{
			name:     "after nested block comment - Windows EOL",
			input:    "(; TODO (; (YOLO) ;) ;)\r\n a",
			expected: []*token{{tokenKeyword, 2, 2, 26, "a"}},
		},
		{
			name:     "white space between parens",
			input:    "( )",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenRParen, 1, 3, 2, ")"}},
		},
		{
			name:  "nested parens",
			input: "(())",
			expected: []*token{
				{tokenLParen, 1, 1, 0, "("},
				{tokenLParen, 1, 2, 1, "("},
				{tokenRParen, 1, 3, 2, ")"},
				{tokenRParen, 1, 4, 3, ")"},
			},
		},
		{
			name:     "empty string",
			input:    `""`,
			expected: []*token{{tokenString, 1, 1, 0, `""`}},
		},
		{
			name:     "unicode string",
			input:    "\"брэд-ЛГТМ\"",
			expected: []*token{{tokenString, 1, 1, 0, "\"брэд-ЛГТМ\""}},
		},
		{
			name:     "string inside tokens with newline",
			input:    "(\"\n\")", // TODO newline char isn't actually allowed unless escaped!
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenString, 1, 2, 1, "\"\n\""}, {tokenRParen, 1, 5, 4, ")"}},
		},
		{
			name:     "unsigned shortest - EOL",
			input:    "1\n",
			expected: []*token{{tokenUN, 1, 1, 0, "1"}},
		},
		{
			name:     "unsigned shortest - EOF",
			input:    "1",
			expected: []*token{{tokenUN, 1, 1, 0, "1"}},
		},
		{
			name:     "unsigned shortest inside tokens",
			input:    "(1)",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenUN, 1, 2, 1, "1"}, {tokenRParen, 1, 3, 2, ")"}},
		},
		{
			name:     "unsigned shortest then string",
			input:    `1"1"`,
			expected: []*token{{tokenUN, 1, 1, 0, "1"}, {tokenString, 1, 2, 1, `"1"`}},
		},
		{
			name:     "unsigned - EOL",
			input:    "123\n",
			expected: []*token{{tokenUN, 1, 1, 0, "123"}},
		},
		{
			name:     "unsigned - EOF",
			input:    "123",
			expected: []*token{{tokenUN, 1, 1, 0, "123"}},
		},
		{
			name:     "unsigned inside tokens",
			input:    "(123)",
			expected: []*token{{tokenLParen, 1, 1, 0, "("}, {tokenUN, 1, 2, 1, "123"}, {tokenRParen, 1, 5, 4, ")"}},
		},
		{
			name:     "unsigned then string",
			input:    `123"123"`,
			expected: []*token{{tokenUN, 1, 1, 0, "123"}, {tokenString, 1, 4, 3, `"123"`}},
		},
		{
			name:     "unsigned then keyword",
			input:    "1a", // whitespace is optional between tokens, and a keyword can be single-character!
			expected: []*token{{tokenUN, 1, 1, 0, "1"}, {tokenKeyword, 1, 2, 1, "a"}},
		},
		{
			name:  "0x80 in block comment",
			input: "(; \000);)",
		},
		{
			name:  "0x80 in block comment unicode",
			input: "(; 私\000);)",
		},
		{
			name:  "0x80 in line comment",
			input: ";; \000",
		},
		{
			name:     "0x80 in string",
			input:    "\" \000\"",
			expected: []*token{{tokenString, 1, 1, 0, "\" \000\""}},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			tokens, e := lexTokens(tc.input)
			require.NoError(t, e)
			require.Equal(t, tc.expected, tokens)
		})
	}
}

func TestLex_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "close paren before open paren",
			input:       []byte(")("),
			expectedErr: "test.wasm:1:1: found ')' before '('",
		},
		{
			name:        "unbalanced nesting",
			input:       []byte("(()"),
			expectedErr: "test.wasm:1:4: expected ')', but reached end of input",
		},
		{
			name:        "open paren at end of input",
			input:       []byte("("),
			expectedErr: "test.wasm:1:1: found '(' at end of input",
		},
		{
			name:        "begin block comment at end of input",
			input:       []byte("(;"),
			expectedErr: "test.wasm:1:3: expected block comment end ';)', but reached end of input",
		},
		{
			name:        "half line comment",
			input:       []byte("; TODO"),
			expectedErr: "test.wasm:1:1: unexpected character ;",
		},
		{
			name:        "open block comment",
			input:       []byte("(; TODO"),
			expectedErr: "test.wasm:1:8: expected block comment end ';)', but reached end of input",
		},
		{
			name:        "close block comment",
			input:       []byte(";) TODO"),
			expectedErr: "test.wasm:1:1: unexpected character ;",
		},
		{
			name:        "unbalanced nested block comment",
			input:       []byte("(; TODO (; (YOLO) ;)"),
			expectedErr: "test.wasm:1:21: expected block comment end ';)', but reached end of input",
		},
		{
			name:        "dangling unicode",
			input:       []byte(" 私"),
			expectedErr: "test.wasm:1:2: expected an ASCII character, not 私",
		},
		{
			name:        "0x80 in block comment",
			input:       []byte("(; \200)"),
			expectedErr: "test.wasm:1:4: found an invalid byte in block comment: 0x80",
		},
		{
			name:        "0x80 in block comment unicode",
			input:       []byte("(; 私\200)"),
			expectedErr: "test.wasm:1:5: found an invalid byte in block comment: 0x80",
		},
		{
			name:        "0x80 in line comment",
			input:       []byte(";; \200"),
			expectedErr: "test.wasm:1:4: found an invalid byte in line comment: 0x80",
		},
		{
			name:        "0x80 in line comment unicode",
			input:       []byte(";; 私\200"),
			expectedErr: "test.wasm:1:5: found an invalid byte in line comment: 0x80",
		},
		{
			name:        "0x80 in string",
			input:       []byte("\" \200\""),
			expectedErr: "test.wasm:1:3: found an invalid byte in string token: 0x80",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, lex("test.wasm", tc.input, noopParseToken), tc.expectedErr)
		})
	}
}

func lexTokens(input string) ([]*token, error) {
	var tokens []*token
	e := lex("test.wasm", []byte(input), func(source []byte, tok tokenType, line, col, beginPos, endPos int) (err error) {
		tokens = append(tokens, &token{tok, line, col, beginPos, string(source[beginPos:endPos])})
		return
	})
	return tokens, e
}

var noopParseToken parseToken = func(source []byte, tok tokenType, beginLine, beginCol, beginPos, endPos int) error {
	return nil
}

func BenchmarkLex(b *testing.B) {
	benchmarks := []struct {
		name string
		data []byte
	}{
		{"example", []byte(exampleWat)},
		{"whitespace chars", []byte("(                        \nmodule)\n")}, // 34 bytes
		{"unicode line comment", []byte("( ;; брэд-ЛГТМ   \nmodule)\n")},     // 28 bytes
		{"unicode block comment", []byte("( (; брэд-ЛГТМ ;)\nmodule)\n")},    // 28 bytes
	}

	for _, bm := range benchmarks {
		b.Run(bm.name+" vs utf8.ValidString", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				utf8.ValidString(string(bm.data))
			}
		})
		// Not a fair comparison as we are only lexing and not writing back %.wasm
		// If possible, we should find a way to isolate only the lexing C functions.
		b.Run(bm.name+" vs wasmtime.Wat2Wasm", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := wasmtime.Wat2Wasm(string(bm.data))
				if err != nil {
					panic(err)
				}
			}
		})
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				err := lex("test.wasm", bm.data, noopParseToken)
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

type token struct {
	tokenType
	line, col, pos int
	value          string
}

// String helps format to allow copy/pasting of expected values
func (t *token) String() string {
	return fmt.Sprintf("{%s, %d, %d, %d, %q}", t.tokenType, t.line, t.col, t.pos, t.value)
}
