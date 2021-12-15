package wat

// token is the set of tokens defined by the WebAssembly Text Format 1.0
// See https://www.w3.org/TR/wasm-core-1/#tokens%E2%91%A0
type tokenType byte

const (
	tokenInvalid tokenType = iota
	// tokenKeyword is a potentially empty sequence of idChar characters prefixed by a lowercase letter.
	//
	// For example, in the below, 'local.get' 'i32.const' and 'i32.lt_s' are keywords:
	//		local.get $y
	//		i32.const 6
	//		i32.lt_s
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-keyword
	tokenKeyword

	// tokenUN is an unsigned integer in decimal or hexadecimal notation, optionally separated by underscores.
	//
	// For example, the following tokens represent the same number: 10
	//		(i32.const 10)
	//		(i32.const 1_0)
	//		(i32.const 0x0a)
	//		(i32.const 0x0_A)
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-int
	tokenUN

	// tokenSN is a signed integer in decimal or hexadecimal notation, optionally separated by underscores.
	//
	// For example, the following tokens represent the same number: 10
	//		(i32.const +10)
	//		(i32.const +1_0)
	//		(i32.const +0x0a)
	//		(i32.const +0x0_A)
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-int
	tokenSN

	// tokenFN represents an IEEE-754 floating point number in decimal or hexadecimal notation, optionally separated by
	// underscores. This also includes special constants for infinity ('inf') and NaN ('nan').
	//
	// For example, the right-hand side of the following S-expressions are all valid floating point tokens:
	//		(f32.const +nan)
	//		(f64.const -nan:0xfffffffffffff)
	//		(f64.const -inf)
	//      (f64.const +0x0.0p0)
	//      (f32.const 0.0e0)
	//		(i32.const +0x0_A)
	//		(f32.const 1.e10)
	//      (f64.const 0x1.fff_fff_fff_fff_fp+1_023)
	//		(f64.const 1.7976931348623157e+308)
	tokenFN

	// tokenString is a UTF-8 sequence enclosed by quotation marks, representing an encoded byte string. A tokenString
	// can contain any character except ASCII control characters, quotation marks ('"') and backslash ('\'): these must
	// be escaped. tokenString characters correspond to UTF-8 encoding except the special case of '\hh', which allows
	// raw bytes expressed as hexadecimal.
	//
	// For example, the following tokens represent the same raw bytes: 0xe298ba0a
	//		(data (i32.const 0) "☺\n")
	//		(data (i32.const 0) "\u{263a}\u{0a}")
	//		(data (i32.const 0) "\e2\98\ba\0a")
	//
	// See https://www.w3.org/TR/wasm-core-1/#strings%E2%91%A0
	tokenString

	// tokenId is a sequence of idChar characters prefixed by a '$':
	//
	// For example, in the below, '$y' is an id:
	//		local.get $y
	//		i32.const 6
	//		i32.lt_s
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-id
	tokenId

	// tokenLParen is a left paren: '('
	tokenLParen

	// tokenRParen is a right paren: ')'
	tokenRParen

	// tokenReserved is a sequence of idChar characters which are neither a tokenId nor a tokenString.
	//
	// For example, '0$y' is a tokenReserved, because it doesn't start with a letter or '$'.
	//
	// See https://www.w3.org/TR/wasm-core-1/#text-reserved
	tokenReserved
)

// tokenNames is index-coordinated with tokenType
var tokenNames = [...]string{
	"tokenInvalid",
	"tokenKeyword",
	"tokenUN",
	"tokenSN",
	"tokenFN",
	"tokenString",
	"tokenId",
	"tokenLParen",
	"tokenRParen",
	"tokenReserved",
}

// String returns the string name of this token.
func (t tokenType) String() string {
	return tokenNames[t]
}

// constants below help format a somewhat readable lookup table that eases identification of tokens.
const (
	// xx is an invalid token start byte
	xx = tokenInvalid
	// xs is the start of tokenString ('"')
	xs = tokenString
	// xi is the start of tokenId ('$')
	xi = tokenId
	// lp is the start of tokenLParen ('(')
	lp = tokenLParen
	// rp is the start of tokenRParen (')')
	rp = tokenRParen
	// un is the start of a tokenUN (or tokenFN)
	un = tokenUN
	// sn is the start of a tokenSN (or tokenFN)
	sn = tokenSN
	// xk is the start of a tokenKeyword
	xk = tokenKeyword
	// xr is the start of tokenReserved (a valid, but not defined above).
	xr = tokenReserved
)

// firstTokenByte is information about the firstTokenByte byte in a token. All expected token starts are ASCII, but we
// switch to avoid a range check.
var firstTokenByte = [256]tokenType{
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0x00-0x0F
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0x10-0x1F
	xx, xr, xs, xr, xi, xr, xr, xr, lp, rp, xr, sn, xx, sn, xr, xr, // 0x20-0x2F
	un, un, un, un, un, un, un, un, un, un, xr, xx, xr, xr, xr, xr, // 0x30-0x3F
	xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, // 0x40-0x4F
	xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xr, xx, xr, xx, xr, xr, // 0x50-0x5F
	xr, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, // 0x60-0x6F
	xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xk, xx, xr, xx, xr, xx, // 0x70-0x7F
}

// idChar is a printable ASCII character that does not contain a space, quotation mark, comma, semicolon, or bracket.
// See https://www.w3.org/TR/wasm-core-1/#text-idchar
var idChar = buildIdChars()

func buildIdChars() (result [256]bool) {
	for i := 0; i < 128; i++ {
		result[i] = _idChar(byte(i))
	}
	return
}

func _idChar(ch byte) bool {
	switch ch {
	case '!':
		fallthrough
	case '#':
		fallthrough
	case '$':
		fallthrough
	case '%':
		fallthrough
	case '&':
		fallthrough
	case '\'':
		fallthrough
	case '*':
		fallthrough
	case '+':
		fallthrough
	case '-':
		fallthrough
	case '.':
		fallthrough
	case '/':
		fallthrough
	case ':':
		fallthrough
	case '<':
		fallthrough
	case '=':
		fallthrough
	case '>':
		fallthrough
	case '?':
		fallthrough
	case '@':
		fallthrough
	case '\\':
		fallthrough
	case '^':
		fallthrough
	case '_':
		fallthrough
	case '`':
		fallthrough
	case '|':
		fallthrough
	case '~':
		return true
	}
	switch {
	case ch >= '0' && ch <= '9':
		fallthrough
	case ch >= 'a' && ch <= 'z':
		fallthrough
	case ch >= 'A' && ch <= 'Z':
		return true
	}
	return false
}
