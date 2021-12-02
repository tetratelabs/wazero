package wat

import (
	"fmt"
	"unicode/utf8"
)

// parseToken allows a parser to inspect a token without necessarily allocating strings
// * source is the underlying byte stream: do not modify this
// * tokenType is the token type
// * beginPos is the byte position in the source where the token begins, inclusive
// * endPos is the byte position in the source where the token ends, exclusive
//
// Returning an error will short-circuit any future invocations.
type parseToken func(source []byte, tok tokenType, beginLine, beginCol, beginPos, endPos int) error

// lex invokes the parser function for each token, the source is exhausted.
//
// Errors from the parser or during tokenization exit early, such as dangling block comments or unexpected characters.
func lex(source []byte, parser parseToken) error {
	length := len(source)
	// p is the position in the source, and a parameter to the parser.
	p := 0
	// line is the line number in the source as determined by unescaped '\n' characters, and a parameter to the parser.
	line := 1
	// col is the UTF-8 aware column number, and a parameter to the parser
	col := 0

	// Block comments, ex. (; comment ;), can span multiple lines and also nest, ex. (; one (; two ;) ).
	// There may be no block comments, but we declare the variable that tracks them here, as it is more efficient vs
	// inline processing.
	blockCommentLevel := 0

	for ; p < length; p = p + 1 {
		b1 := source[p]

		// The spec does not consider newlines apart from '\n'. Notably, a bare '\r' is not a newline here.
		// See https://www.w3.org/TR/wasm-core-1/#text-comment
		if b1 == '\n' {
			line = line + 1
			col = 0
			continue // next line
		}

		// Otherwise, advance the column regardless of whether we are whitespace or not
		col = col + 1                              // the current character is at least one byte long
		if b1 == ' ' || b1 == '\t' || b1 == '\r' { // fast path ASCII whitespace
			continue // next whitespace
		}

		// Handle comments, noting they can be nested
		switch b1 {
		case '(':
			peekPos := p + 1
			if peekPos < length && source[peekPos] == ';' { // block comment
				p = peekPos // continue after "(;"
				col = col + 1
				blockCommentLevel = blockCommentLevel + 1
				continue
			}
		case ';': // possible line comment or block comment end
			peekPos := p + 1
			if peekPos < length {
				b2 := source[peekPos]
				if blockCommentLevel > 0 && b2 == ')' {
					p = peekPos // continue after ";)"
					col = col + 1
					blockCommentLevel = blockCommentLevel - 1
					continue
				}

				if b2 == ';' { // line comment
					// Start after ";;" and run until the end. Note UTF-8 (multi-byte) characters are allowed.
					peekPos = peekPos + 1
					col = col + 1

					for peekPos < length {
						peeked := source[peekPos]
						if peeked == '\n' {
							break // EOL bookkeeping will proceed on the next iteration
						}

						col = col + 1
						s := utf8Size[peeked] // While unlikely, it is possible the byte peeked is invalid unicode
						if s == 0 {
							return fmt.Errorf("%d:%d found an invalid byte in line comment: 0x%x", line, col, peeked)
						}
						peekPos = peekPos + s
					}

					p = peekPos - 1 // at the '\n'
					continue        // end of line comment
				}
			}
		}

		// non-ASCII is only supported in comments. Check UTF-8 size as we may need to set position > column!
		if blockCommentLevel > 0 {
			s := utf8Size[b1] // While unlikely, it is possible the current byte is invalid unicode
			if s == 0 {
				return fmt.Errorf("%d:%d found an invalid byte in block comment: 0x%x", line, col, b1)
			}
			p = p + s - 1 // -1 as the for loop will + 1: This optimizes speed of tokenization over block comments.
			continue
		}

		// One design-affecting constraint is that all tokens begin and end with a 7-bit ASCII character. This
		// simplifies line and column counting and how to detect the end of a token.
		//
		// Note: Even though string allows unicode contents it is enclosed by ASCII double-quotes (").
		//
		// See https://www.w3.org/TR/wasm-core-1/#characters%E2%91%A0

		// While many tokens can be single character only parens must be: Handle these more cheaply.
		switch b1 {
		case '(':
			if e := parser(source, tokenLParen, line, col, p, p+1); e != nil {
				return e
			}
			continue
		case ')':
			if e := parser(source, tokenRParen, line, col, p, p+1); e != nil {
				return e
			}
			continue
		}

		// Track positions passed to the parser
		// beginLine == line because no token is allowed to include an unescaped '\n'
		beginCol := col  // the start column of the token (fixed)
		beginPos := p    // the start position of the token (fixed)
		peekPos := p + 1 // when finished scanning, this becomes endPos (the position after the token).

		tok := firstTokenByte[b1]
		switch tok {
		case tokenSN: // min 2 bytes for sign and number; ambiguous: could be tokenFN
			return fmt.Errorf("%d:%d TODO: signed", line, col)
		case tokenUN: // min 1 byte; ambiguous when >=3 bytes as could be tokenFN
			if peekPos < length {
				peeked := source[peekPos]
				if peeked == 'x' {
					return fmt.Errorf("%d:%d TODO: hex", line, col)
				}
			loop:
				// Start after the number and run until the end. Note all allowed characters are single byte.
				for ; peekPos < length; peekPos = peekPos + 1 {
					switch source[peekPos] {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '_':
						p = peekPos
						col = col + 1
					default:
						break loop // end of this token (or malformed, which the next loop will notice)
					}
				}
			}
		case tokenString: // min 2 bytes for empty string ("")
			hitQuote := false
			// Start at the second character and run until the end. Note UTF-8 (multi-byte) characters are allowed.
			for peekPos < length {
				peeked := source[peekPos]
				if peeked == '"' { // TODO: escaping and banning disallowed characters like newlines.
					hitQuote = true
					break
				}

				col = col + 1
				s := utf8Size[peeked] // While unlikely, it is possible the current byte is invalid unicode
				if s == 0 {
					return fmt.Errorf("%d:%d found an invalid byte in string token: 0x%x", line, col, peeked)
				}
				peekPos = peekPos + s
			}

			if !hitQuote {
				return fmt.Errorf("%d:%d expected end quote", line, col)
			}

			// set the position to after the quote
			p = peekPos
			peekPos = peekPos + 1
			col = col + 1
		case tokenKeyword, tokenId, tokenReserved: // min 1 byte; end with zero or more idChar
			// Start after the first character and run until the end. Note all allowed characters are single byte.
			for ; peekPos < length; peekPos = peekPos + 1 {
				if !idChar[source[peekPos]] {
					break // end of this token (or malformed, which the next loop will notice)
				}
				col = col + 1
			}
			p = peekPos - 1
		default:
			if b1 > 0x7F { // non-ASCII
				r, _ := utf8.DecodeRune(source[line:])
				return fmt.Errorf("%d:%d expected an ASCII character, not %s", line, col, string(r))
			}
			return fmt.Errorf("%d:%d unexpected character %s", line, col, string(b1))
		}

		// Unsigned floating-point constants for infinity or canonical NaN (not a number) clash with keyword
		// representation. For example, "nan" and "inf" are floating-point constants, while "nano" and "info" are
		// possible keywords. See https://www.w3.org/TR/wasm-core-1/#floating-point%E2%91%A6
		//
		// TODO: Ex. inf nan nan:0xfffffffffffff or nan:0x400000

		if e := parser(source, tok, line, beginCol, beginPos, peekPos); e != nil {
			return e
		}
	}

	if blockCommentLevel > 0 {
		return fmt.Errorf("%d:%d expected block comment end ';)'", line, col)
	}
	return nil // EOF
}

// utf8Size returns the size of the UTF-8 rune based on its first byte, or zero
var utf8Size = [256]int{
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x00-0x0F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x10-0x1F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x20-0x2F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x30-0x3F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x40-0x4F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x50-0x5F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x60-0x6F
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 0x70-0x7F
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0x80-0x8F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0x90-0x9F
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xA0-0xAF
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xB0-0xBF
	0, 0, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, // 0xC0-0xCF
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, // 0xD0-0xDF
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, // 0xE0-0xEF
	4, 4, 4, 4, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0xF0-0xFF
}
