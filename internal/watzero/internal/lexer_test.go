package internal

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
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
	require.Equal(t, []*token{
		{tokenLParen, 1, 1, "("},
		{tokenKeyword, 1, 2, "module"},
		{tokenLParen, 3, 3, "("},
		{tokenKeyword, 3, 4, "memory"},
		{tokenUN, 3, 11, "1"},
		{tokenRParen, 3, 12, ")"},
		{tokenLParen, 4, 3, "("},
		{tokenKeyword, 4, 4, "func"},
		{tokenID, 4, 9, "$main"},
		{tokenLParen, 4, 15, "("},
		{tokenKeyword, 4, 16, "local"},
		{tokenKeyword, 4, 22, "i32"},
		{tokenKeyword, 4, 26, "i32"},
		{tokenKeyword, 4, 30, "i32"},
		{tokenKeyword, 4, 34, "i32"},
		{tokenRParen, 4, 37, ")"},
		{tokenLParen, 5, 5, "("},
		{tokenKeyword, 5, 6, "set_local"},
		{tokenUN, 5, 16, "0"},
		{tokenLParen, 5, 18, "("},
		{tokenKeyword, 5, 19, "i32.const"},
		{tokenUN, 5, 29, "0"},
		{tokenRParen, 5, 30, ")"},
		{tokenRParen, 5, 31, ")"},
		{tokenLParen, 6, 5, "("},
		{tokenKeyword, 6, 6, "set_local"},
		{tokenUN, 6, 16, "1"},
		{tokenLParen, 6, 18, "("},
		{tokenKeyword, 6, 19, "i32.const"},
		{tokenUN, 6, 29, "1"},
		{tokenRParen, 6, 30, ")"},
		{tokenRParen, 6, 31, ")"},
		{tokenLParen, 7, 5, "("},
		{tokenKeyword, 7, 6, "set_local"},
		{tokenUN, 7, 16, "2"},
		{tokenLParen, 7, 18, "("},
		{tokenKeyword, 7, 19, "i32.const"},
		{tokenUN, 7, 29, "1"},
		{tokenRParen, 7, 30, ")"},
		{tokenRParen, 7, 31, ")"},
		{tokenLParen, 8, 5, "("},
		{tokenKeyword, 8, 6, "set_local"},
		{tokenUN, 8, 16, "3"},
		{tokenLParen, 8, 18, "("},
		{tokenKeyword, 8, 19, "i32.const"},
		{tokenUN, 8, 29, "0"},
		{tokenRParen, 8, 30, ")"},
		{tokenRParen, 8, 31, ")"},
		{tokenLParen, 9, 5, "("},
		{tokenKeyword, 9, 6, "block"},
		{tokenLParen, 10, 5, "("},
		{tokenKeyword, 10, 6, "loop"},
		{tokenLParen, 11, 9, "("},
		{tokenKeyword, 11, 10, "br_if"},
		{tokenUN, 11, 16, "1"},
		{tokenLParen, 11, 18, "("},
		{tokenKeyword, 11, 19, "i32.gt_s"},
		{tokenLParen, 11, 28, "("},
		{tokenKeyword, 11, 29, "get_local"},
		{tokenUN, 11, 39, "0"},
		{tokenRParen, 11, 40, ")"},
		{tokenLParen, 11, 42, "("},
		{tokenKeyword, 11, 43, "i32.const"},
		{tokenUN, 11, 53, "5"},
		{tokenRParen, 11, 54, ")"},
		{tokenRParen, 11, 55, ")"},
		{tokenRParen, 11, 56, ")"},
		{tokenLParen, 12, 9, "("},
		{tokenKeyword, 12, 10, "set_local"},
		{tokenUN, 12, 20, "3"},
		{tokenLParen, 12, 22, "("},
		{tokenKeyword, 12, 23, "get_local"},
		{tokenUN, 12, 33, "2"},
		{tokenRParen, 12, 34, ")"},
		{tokenRParen, 12, 35, ")"},
		{tokenLParen, 13, 9, "("},
		{tokenKeyword, 13, 10, "set_local"},
		{tokenUN, 13, 20, "2"},
		{tokenLParen, 13, 22, "("},
		{tokenKeyword, 13, 23, "i32.add"},
		{tokenLParen, 13, 31, "("},
		{tokenKeyword, 13, 32, "get_local"},
		{tokenUN, 13, 42, "2"},
		{tokenRParen, 13, 43, ")"},
		{tokenLParen, 13, 45, "("},
		{tokenKeyword, 13, 46, "get_local"},
		{tokenUN, 13, 56, "1"},
		{tokenRParen, 13, 57, ")"},
		{tokenRParen, 13, 58, ")"},
		{tokenRParen, 13, 59, ")"},
		{tokenLParen, 14, 9, "("},
		{tokenKeyword, 14, 10, "set_local"},
		{tokenUN, 14, 20, "1"},
		{tokenLParen, 14, 22, "("},
		{tokenKeyword, 14, 23, "get_local"},
		{tokenUN, 14, 33, "3"},
		{tokenRParen, 14, 34, ")"},
		{tokenRParen, 14, 35, ")"},
		{tokenLParen, 15, 9, "("},
		{tokenKeyword, 15, 10, "set_local"},
		{tokenUN, 15, 20, "0"},
		{tokenLParen, 15, 22, "("},
		{tokenKeyword, 15, 23, "i32.add"},
		{tokenLParen, 15, 31, "("},
		{tokenKeyword, 15, 32, "get_local"},
		{tokenUN, 15, 42, "0"},
		{tokenRParen, 15, 43, ")"},
		{tokenLParen, 15, 45, "("},
		{tokenKeyword, 15, 46, "i32.const"},
		{tokenUN, 15, 56, "1"},
		{tokenRParen, 15, 57, ")"},
		{tokenRParen, 15, 58, ")"},
		{tokenRParen, 15, 59, ")"},
		{tokenLParen, 16, 9, "("},
		{tokenKeyword, 16, 10, "br"},
		{tokenUN, 16, 13, "0"},
		{tokenRParen, 16, 14, ")"},
		{tokenRParen, 17, 5, ")"},
		{tokenRParen, 18, 5, ")"},
		{tokenLParen, 19, 5, "("},
		{tokenKeyword, 19, 6, "i32.store"},
		{tokenLParen, 19, 16, "("},
		{tokenKeyword, 19, 17, "i32.const"},
		{tokenUN, 19, 27, "0"},
		{tokenRParen, 19, 28, ")"},
		{tokenLParen, 19, 30, "("},
		{tokenKeyword, 19, 31, "get_local"},
		{tokenUN, 19, 41, "2"},
		{tokenRParen, 19, 42, ")"},
		{tokenRParen, 19, 43, ")"},
		{tokenRParen, 20, 3, ")"},
		{tokenLParen, 21, 3, "("},
		{tokenKeyword, 21, 4, "start"},
		{tokenID, 21, 10, "$main"},
		{tokenRParen, 21, 15, ")"},
		{tokenLParen, 22, 3, "("},
		{tokenKeyword, 22, 4, "data"},
		{tokenLParen, 22, 9, "("},
		{tokenKeyword, 22, 10, "i32.const"},
		{tokenUN, 22, 20, "0"},
		{tokenRParen, 22, 21, ")"},
		{tokenString, 22, 23, "\"0000\""},
		{tokenRParen, 22, 29, ")"},
		{tokenRParen, 23, 1, ")"},
	}, lexTokens(t, exampleWat))
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
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenRParen, 1, 2, ")"}},
		},
		{
			name:     "shortest keywords",
			input:    "a z",
			expected: []*token{{tokenKeyword, 1, 1, "a"}, {tokenKeyword, 1, 3, "z"}},
		},
		{
			name:     "shortest tokens - EOL",
			input:    "(a)\n",
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenKeyword, 1, 2, "a"}, {tokenRParen, 1, 3, ")"}},
		},
		{
			name:     "only tokens",
			input:    "(module)",
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenKeyword, 1, 2, "module"}, {tokenRParen, 1, 8, ")"}},
		},
		{
			name:  "only white space characters",
			input: " \t\r\n",
		},
		{
			name:     "after white space characters - EOL",
			input:    " \t\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after white space characters - Windows EOL",
			input:    " \t\r\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
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
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "double line comment",
			input:    ";; TODO\n;; YOLO\na",
			expected: []*token{{tokenKeyword, 3, 1, "a"}},
		},
		{
			name:     "after unicode line comment",
			input:    ";; брэд-ЛГТМ\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after line comment - Windows EOL",
			input:    ";; TODO\r\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after redundant line comment",
			input:    ";;;; TODO\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after line commenting out block comment",
			input:    ";; TODO (; ;)\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after line commenting out open block comment",
			input:    ";; TODO (;\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
		},
		{
			name:     "after line commenting out close block comment",
			input:    ";; TODO ;)\na",
			expected: []*token{{tokenKeyword, 2, 1, "a"}},
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
			expected: []*token{{tokenKeyword, 1, 21, "a"}},
		},
		{
			name:     "double block comment - EOL",
			input:    "(; TODO ;)\n(; YOLO ;)\na",
			expected: []*token{{tokenKeyword, 3, 1, "a"}},
		},
		{
			name:     "after block comment",
			input:    "(; TODO ;)a",
			expected: []*token{{tokenKeyword, 1, 11, "a"}},
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
			expected: []*token{{tokenKeyword, 1, 24, "a"}},
		},
		{
			name:     "after nested block comment - EOL",
			input:    "(; TODO (; (YOLO) ;) ;)\n a",
			expected: []*token{{tokenKeyword, 2, 2, "a"}},
		},
		{
			name:     "after nested block comment - Windows EOL",
			input:    "(; TODO (; (YOLO) ;) ;)\r\n a",
			expected: []*token{{tokenKeyword, 2, 2, "a"}},
		},
		{
			name:     "white space between parens",
			input:    "( )",
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenRParen, 1, 3, ")"}},
		},
		{
			name:  "nested parens",
			input: "(())",
			expected: []*token{
				{tokenLParen, 1, 1, "("},
				{tokenLParen, 1, 2, "("},
				{tokenRParen, 1, 3, ")"},
				{tokenRParen, 1, 4, ")"},
			},
		},
		{
			name:     "empty string",
			input:    `""`,
			expected: []*token{{tokenString, 1, 1, `""`}},
		},
		{
			name:     "unicode string",
			input:    "\"брэд-ЛГТМ\"",
			expected: []*token{{tokenString, 1, 1, "\"брэд-ЛГТМ\""}},
		},
		{
			name:     "string inside tokens with newline",
			input:    "(\"\n\")", // TODO newline char isn't actually allowed unless escaped!
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenString, 1, 2, "\"\n\""}, {tokenRParen, 1, 5, ")"}},
		},
		{
			name:     "unsigned shortest - EOL",
			input:    "1\n",
			expected: []*token{{tokenUN, 1, 1, "1"}},
		},
		{
			name:     "unsigned shortest - EOF",
			input:    "1",
			expected: []*token{{tokenUN, 1, 1, "1"}},
		},
		{
			name:     "unsigned shortest inside tokens",
			input:    "(1)",
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenUN, 1, 2, "1"}, {tokenRParen, 1, 3, ")"}},
		},
		{
			name:     "unsigned shortest then string",
			input:    `1"1"`,
			expected: []*token{{tokenUN, 1, 1, "1"}, {tokenString, 1, 2, `"1"`}},
		},
		{
			name:     "unsigned - EOL",
			input:    "123\n",
			expected: []*token{{tokenUN, 1, 1, "123"}},
		},
		{
			name:     "unsigned - EOF",
			input:    "123",
			expected: []*token{{tokenUN, 1, 1, "123"}},
		},
		{
			name:     "unsigned inside tokens",
			input:    "(123)",
			expected: []*token{{tokenLParen, 1, 1, "("}, {tokenUN, 1, 2, "123"}, {tokenRParen, 1, 5, ")"}},
		},
		{
			name:     "unsigned then string",
			input:    `123"123"`,
			expected: []*token{{tokenUN, 1, 1, "123"}, {tokenString, 1, 4, `"123"`}},
		},
		{
			name:     "unsigned then keyword",
			input:    "1a", // whitespace is optional between tokens, and a keyword can be single-character!
			expected: []*token{{tokenUN, 1, 1, "1"}, {tokenKeyword, 1, 2, "a"}},
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
			expected: []*token{{tokenString, 1, 1, "\" \000\""}},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, lexTokens(t, tc.input))
		})
	}
}

func TestLex_Errors(t *testing.T) {
	tests := []struct {
		name                      string
		parser                    tokenParser
		input                     []byte
		expectedLine, expectedCol uint32
		expectedErr               string
	}{
		{
			name:         "close paren before open paren",
			input:        []byte(")("),
			expectedLine: 1,
			expectedCol:  1,
			expectedErr:  "found ')' before '('",
		},
		{
			name:         "unbalanced nesting",
			input:        []byte("(()"),
			expectedLine: 1,
			expectedCol:  4,
			expectedErr:  "expected ')', but reached end of input",
		},
		{
			name:         "open paren at end of input",
			input:        []byte("("),
			expectedLine: 1,
			expectedCol:  1,
			expectedErr:  "found '(' at end of input",
		},
		{
			name:         "begin block comment at end of input",
			input:        []byte("(;"),
			expectedLine: 1,
			expectedCol:  3,
			expectedErr:  "expected block comment end ';)', but reached end of input",
		},
		{
			name:         "half line comment",
			input:        []byte("; TODO"),
			expectedLine: 1,
			expectedCol:  1,
			expectedErr:  "unexpected character ;",
		},
		{
			name:         "open block comment",
			input:        []byte("(; TODO"),
			expectedLine: 1,
			expectedCol:  8,
			expectedErr:  "expected block comment end ';)', but reached end of input",
		},
		{
			name:         "close block comment",
			input:        []byte(";) TODO"),
			expectedLine: 1,
			expectedCol:  1,

			expectedErr: "unexpected character ;",
		},
		{
			name:         "unbalanced nested block comment",
			input:        []byte("(; TODO (; (YOLO) ;)"),
			expectedLine: 1,
			expectedCol:  21,
			expectedErr:  "expected block comment end ';)', but reached end of input",
		},
		{
			name:         "dangling unicode",
			input:        []byte(" 私"),
			expectedLine: 1,
			expectedCol:  2,
			expectedErr:  "expected an ASCII character, not 私",
		},
		{
			name:         "truncated string",
			input:        []byte("\"hello"),
			expectedLine: 1,
			expectedCol:  6,
			expectedErr:  "expected end quote",
		},
		{
			name:         "0x80 in block comment",
			input:        []byte("(; \200)"),
			expectedLine: 1,
			expectedCol:  4,
			expectedErr:  "found an invalid byte in block comment: 0x80",
		},
		{
			name:         "0x80 in block comment unicode",
			input:        []byte("(; 私\200)"),
			expectedLine: 1,
			expectedCol:  5,
			expectedErr:  "found an invalid byte in block comment: 0x80",
		},
		{
			name:         "0x80 in line comment",
			input:        []byte(";; \200"),
			expectedLine: 1,
			expectedCol:  4,
			expectedErr:  "found an invalid byte in line comment: 0x80",
		},
		{
			name:         "0x80 in line comment unicode",
			input:        []byte(";; 私\200"),
			expectedLine: 1,
			expectedCol:  5,
			expectedErr:  "found an invalid byte in line comment: 0x80",
		},
		{
			name:         "0x80 in string",
			input:        []byte("\" \200\""),
			expectedLine: 1,
			expectedCol:  3,
			expectedErr:  "found an invalid byte in string token: 0x80",
		},
		{
			name:         "parser error: lParen",
			input:        []byte(" (module)"),
			parser:       (&errorOnTokenParser{tokenLParen}).parse,
			expectedLine: 1,
			expectedCol:  2,
			expectedErr:  "unexpected '('",
		},
		{
			name:         "parser error: keyword",
			input:        []byte(" (module)"),
			parser:       (&errorOnTokenParser{tokenKeyword}).parse,
			expectedLine: 1,
			expectedCol:  3,
			expectedErr:  "unexpected keyword: module",
		},
		{
			name:         "parser error: rParen",
			input:        []byte(" (module)"),
			parser:       (&errorOnTokenParser{tokenRParen}).parse,
			expectedLine: 1,
			expectedCol:  9,
			expectedErr:  "unexpected ')'",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			parser := tc.parser
			if parser == nil {
				parser = parseNoop
			}
			line, col, err := lex(parser, tc.input)
			require.Equal(t, tc.expectedLine, line)
			require.Equal(t, tc.expectedCol, col)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func lexTokens(t *testing.T, input string) []*token {
	p := &collectTokenParser{}
	line, col, err := lex(p.parse, []byte(input))
	require.NoError(t, err, "%d:%d: %s", line, col, err)
	return p.tokens
}

type errorOnTokenParser struct{ tok tokenType }

func (e *errorOnTokenParser) parse(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok != e.tok {
		return e.parse, nil
	}
	return parseErr(tok, tokenBytes, line, col)
}

type collectTokenParser struct{ tokens []*token }

func (c *collectTokenParser) parse(tok tokenType, tokenUTF8 []byte, line, col uint32) (tokenParser, error) {
	c.tokens = append(c.tokens, &token{tokenType: tok, line: line, col: col, token: string(tokenUTF8)})
	return c.parse, nil
}

type collectTokenTypeParser struct{ tokenTypes []tokenType }

func (c *collectTokenTypeParser) parse(tok tokenType, _ []byte, _, _ uint32) (tokenParser, error) {
	c.tokenTypes = append(c.tokenTypes, tok)
	return c.parse, nil
}

type noopTokenParser struct{}

func (n *noopTokenParser) parse(_ tokenType, _ []byte, _, _ uint32) (tokenParser, error) {
	return n.parse, nil
}

var parseNoop = (&noopTokenParser{}).parse

type errTokenParser struct{}

func (n *errTokenParser) parse(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	return nil, unexpectedToken(tok, tokenBytes)
}

var parseErr = (&errTokenParser{}).parse

type skipTokenParser struct {
	count uint32
	next  tokenParser
}

func (s *skipTokenParser) parse(_ tokenType, _ []byte, _, _ uint32) (tokenParser, error) {
	s.count--
	if s.count == 0 {
		return s.next, nil
	}
	return s.parse, nil
}

// skipTokens is a hack because lex tracks parens, so they may need to be skipped. Also, some parsers need to skip past
// a field name.
func skipTokens(count uint32, next tokenParser) tokenParser {
	out := &skipTokenParser{count: count, next: next}
	return out.parse
}

type token struct {
	tokenType
	line, col uint32
	token     string
}

// String helps format to allow copy/pasting of expected values
func (t *token) String() string {
	return fmt.Sprintf("{%s, %d, %d, %q}", t.tokenType, t.line, t.col, t.token)
}
