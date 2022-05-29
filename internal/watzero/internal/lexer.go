package internal

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// tokenParser parses the current token and returns a parser for the next.
//
// * tokenType is the token type
// * tokenBytes are the UTF-8 bytes representing the token. Do not modify this.
// * line is the source line number determined by unescaped '\n' characters.
// * col is the UTF-8 column number.
//
// Returning an error will short-circuit any future invocations.
//
// Note: Do not include the line and column number in a parsing error as that will be attached automatically. Line and
// column are here for storing the source location, such as for use in runtime stack traces.
type tokenParser func(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error)

// TODO: since S-expressions are common and also multiple nesting levels in fields, ex. (import (func)), think about a
// special result of popCount which pops one or two RParens. This could inline skipping parens, which have no error
// possibility unless there are extra tokens.

var (
	constantLParen = []byte{'('}
	constantRParen = []byte{')'}
)

// lex invokes the parser function for the given source. This function returns when the source is exhausted or an error
// occurs.
//
// Here's a description of the return values:
// * line is the source line number determined by unescaped '\n' characters of the error or EOF
// * col is the UTF-8 column number of the error or EOF
// * err is an error invoking the parser, dangling block comments or unexpected characters.
func lex(parser tokenParser, source []byte) (line, col uint32, err error) {
	// i is the source index to begin reading, inclusive.
	i := 0
	// end is the source index to stop reading, exclusive.
	end := len(source)
	line = 1
	col = 1

	// Web assembly expressions are grouped by parenthesis, even the minimal example "(module)". We track nesting level
	// to help report problems instead of bubbling to the parser layer.
	parenDepth := 0

	// Block comments, ex. (; comment ;), can span multiple lines and also nest, ex. (; one (; two ;) ).
	// There may be no block comments, but we declare the variable that tracks them here, as it is more efficient vs
	// inline processing.
	blockCommentDepth := 0

	for ; i < end; i, col = i+1, col+1 {
		b1 := source[i]

		// The spec does not consider newlines apart from '\n'. Notably, a bare '\r' is not a newline here.
		// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-comment
		if b1 == '\n' {
			line++
			col = 0  // for loop will + 1
			continue // next line
		}

		if b1 == ' ' || b1 == '\t' || b1 == '\r' { // fast path ASCII whitespace
			continue // next whitespace
		}

		// Handle parens and comments, noting block comments, ex. "(; look! ;)", can be nested.
		switch b1 {
		case '(':
			peek := i + 1
			if peek == end { // invalid regardless of block comment or not. nothing opens at EOF!
				return line, col, errors.New("found '(' at end of input")
			}
			if source[peek] == ';' { // next block comment
				i = peek // continue after "(;"
				col++
				blockCommentDepth++
				continue
			} else if blockCommentDepth == 0 { // Fast path left paren token at the expense of code duplication.
				if parser, err = parser(tokenLParen, constantLParen, line, col); err != nil {
					return line, col, err
				}
				parenDepth++
				continue
			}
		case ')':
			if blockCommentDepth == 0 { // Fast path right paren token at the expense of code duplication.
				if parenDepth == 0 {
					return line, col, errors.New("found ')' before '('")
				}
				if parser, err = parser(tokenRParen, constantRParen, line, col); err != nil {
					return line, col, err
				}
				parenDepth--
				continue
			}
		case ';': // possible line comment or block comment end
			peek := i + 1
			if peek < end {
				b2 := source[peek]
				if blockCommentDepth > 0 && b2 == ')' {
					i = peek // continue after ";)"
					col++
					blockCommentDepth--
					continue
				}

				if b2 == ';' { // line comment
					// Start after ";;" and run until the end. Note UTF-8 (multi-byte) characters are allowed.
					peek++
					col++

				LineComment:
					for peek < end {
						peeked := source[peek]
						if peeked == '\n' {
							break LineComment // EOL bookkeeping will proceed on the next iteration
						}

						col++
						s := utf8Size[peeked] // While unlikely, it is possible the byte peeked is invalid unicode
						if s == 0 {
							return line, col, fmt.Errorf("found an invalid byte in line comment: 0x%x", peeked)
						}
						peek = peek + s
					}

					// -1 because for loop will + 1: This optimizes speed of tokenization over line comments.
					i = peek - 1 // at the '\n'
					continue     // end of line comment
				}
			}
		}

		// non-ASCII is only supported in comments. Check UTF-8 size as we may need to set position > column!
		if blockCommentDepth > 0 {
			s := utf8Size[b1] // While unlikely, it is possible the current byte is invalid unicode
			if s == 0 {
				return line, col, fmt.Errorf("found an invalid byte in block comment: 0x%x", b1)
			}
			i = i + s - 1 // -1 because for loop will + 1: This optimizes speed of tokenization over block comments.
			continue
		}

		// One design-affecting constraint is that all tokens begin and end with a 7-bit ASCII character. This
		// simplifies line and column counting and how to detect the end of a token.
		//
		// Note: Even though string allows unicode contents it is enclosed by ASCII double-quotes (").
		//
		// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#characters%E2%91%A0

		tok := firstTokenByte[b1]
		// Track positions passed to the parser
		b := i        // the start position of the token (fixed)
		peek := i + 1 // when finished scanning, this becomes end (the position after the token).
		// line == line because no token is allowed to include an unescaped '\n'
		c := col // the start column of the token (fixed)

		switch tok {
		// case tokenLParen, tokenRParen: // min/max 1 byte
		case tokenSN: // min 2 bytes for sign and number; ambiguous: could be tokenFN
			return line, c, errors.New("TODO: signed")
		case tokenUN: // min 1 byte; ambiguous when >=3 bytes as could be tokenFN
			if peek < end {
				peeked := source[peek]
				if peeked == 'x' {
					return line, col, errors.New("TODO: hex")
				}
			Number:
				// Start after the number and run until the end. Note all allowed characters are single byte.
				for ; peek < end; peek++ {
					switch source[peek] {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '_':
						i = peek
						col++
					default:
						break Number // end of this token (or malformed, which the next loop will notice)
					}
				}
			}
		case tokenString: // min 2 bytes for empty string ("")
			hitQuote := false
			// Start at the second character and run until the end. Note UTF-8 (multi-byte) characters are allowed.
		String:
			for peek < end {
				peeked := source[peek]
				if peeked == '"' { // TODO: escaping and banning disallowed characters like newlines.
					hitQuote = true
					break String
				}

				col++
				s := utf8Size[peeked] // While unlikely, it is possible the current byte is invalid unicode
				if s == 0 {
					return line, col, fmt.Errorf("found an invalid byte in string token: 0x%x", peeked)
				}
				peek = peek + s
			}

			if !hitQuote {
				return line, col, errors.New("expected end quote")
			}

			i = peek
			// set the position to after the quote
			peek++
			col++
		case tokenKeyword, tokenID, tokenReserved: // min 1 byte; end with zero or more idChar
			// Start after the first character and run until the end. Note all allowed characters are single byte.
		IdChars:
			for ; peek < end; peek++ {
				if !idChar[source[peek]] {
					break IdChars // end of this token (or malformed, which the next loop will notice)
				}
				col++
			}
			i = peek - 1
		default:
			if b1 > 0x7F { // non-ASCII
				r, _ := utf8.DecodeRune(source[line:])
				return line, col, fmt.Errorf("expected an ASCII character, not %s", string(r))
			}
			return line, col, fmt.Errorf("unexpected character %s", string(b1))
		}

		// Unsigned floating-point constants for infinity or canonical NaN (not a number) clash with keyword
		// representation. For example, "nan" and "inf" are floating-point constants, while "nano" and "info" are
		// possible keywords. See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#floating-point%E2%91%A6
		//
		// TODO: Ex. inf nan nan:0xfffffffffffff or nan:0x400000

		if parser, err = parser(tok, source[b:peek], line, c); err != nil {
			return line, c, err
		}
	}

	if blockCommentDepth > 0 {
		return line, col, errors.New("expected block comment end ';)', but reached end of input")
	}
	if parenDepth > 0 {
		return line, col, errors.New("expected ')', but reached end of input")
	}
	return line, col, nil
}

// utf8Size returns the size of the UTF-8 rune based on its first byte, or zero.
//
// Note: The null byte (0x00) is here as it is valid in string tokens and comments. See WebAssembly/spec#1372
//
// Note: We don't validate the subsequent bytes make a well-formed UTF-8 rune intentionally for performance and to keep
// lexing allocation free. Meanwhile, the impact is that we might skip over malformed bytes.
var utf8Size = [256]int{
	// 1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x00-0x0F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x10-0x1F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x20-0x2F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x30-0x3F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x40-0x4F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x50-0x5F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x60-0x6F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x70-0x7F
	// 1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0x80-0x8F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0x90-0x9F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xA0-0xAF
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xB0-0xBF
	0, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, // 0xC0-0xCF
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, // 0xD0-0xDF
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, // 0xE0-0xEF
	4, 4, 4, 4, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xF0-0xFF
}
