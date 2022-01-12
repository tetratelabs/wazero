package wat

import (
	"strconv"
)

// decodeUint32 decodes an uint32 from a tokenUN.
//
// Note: Bit length interpretation is not defined at the lexing layer, so this may fail on overflow due to invalid
// length known at the parsing layer.
//
// Note: This is similar to, but cannot use strconv.Atoi because WebAssembly allows underscore characters in numeric
// representation. See https://www.w3.org/TR/wasm-core-1/#integers%E2%91%A6
func decodeUint32(tokenBytes []byte) (uint32, error) { // TODO: hex
	// The max ASCII length of a uint32 is 10 (length of 4294967295). If we are at that length we can overflow.
	//
	// Note: There's chance we are at length 10 due to underscores, but easier to take the slow path for either reason.
	if len(tokenBytes) > 9 {
		v, err := decodeUint64(tokenBytes)
		if err != nil {
			return 0, err
		}
		if v > 0xffffffff {
			return 0, &strconv.NumError{Func: "decodeU32", Num: string(tokenBytes), Err: strconv.ErrRange}
		}
		return uint32(v), nil
	}

	// If we are here, we know we cannot overflow uint32. The only valid characters in tokenUN are 0-9 and underscore.
	var n uint32
	for _, ch := range tokenBytes {
		if ch == '_' {
			continue
		}
		n = n*10 + uint32(ch-'0')
	}
	return n, nil
}

// decodeUint64 is like decodeUint32, but for uint64
func decodeUint64(tokenBytes []byte) (uint64, error) { // TODO: hex
	// The max ASCII length of a uint64 is 20 (length of 18446744073709551615). If we are at that length we can overflow.
	//
	// Note: There's chance we are at length 20 due to underscores, but easier to take the slow path for either reason.
	if len(tokenBytes) > 19 {
		return decodeUint64SlowPath(tokenBytes)
	}

	// If we are here, we know we cannot overflow uint64. The only valid characters in tokenUN are 0-9 and underscore.
	var n uint64
	for _, ch := range tokenBytes {
		if ch == '_' {
			continue
		}
		n = n*10 + uint64(ch-'0')
	}
	return n, nil
}

// decodeUint64SlowPath implements decodeUint64 for the slow path when there's a chance the result cannot fit.
// Notably, this strips underscore characters first, so that the string length can be used to assess overflows.
func decodeUint64SlowPath(tokenBytes []byte) (uint64, error) {
	// We have a possible overflow, but won't know for sure due to underscores until we strip them. This strips any
	// underscores from the token in-place.
	n := 0
	for _, b := range tokenBytes {
		if b != '_' {
			tokenBytes[n] = b
			n++
		}
	}
	tokenBytes = tokenBytes[:n]

	// Now, we know there are only numbers and no underscores. This means the ASCII length is insightful.
	switch len(tokenBytes) {
	case 19: // cannot overflow
		return decodeUint64(tokenBytes)
	case 20: // only overflows depending on the last number
		first19, err := decodeUint64(tokenBytes[0:19])
		if err != nil {
			return 0, err // impossible unless someone used this with an unvalidated token
		}
		last := uint64(tokenBytes[19] - '0')

		// Remember the largest uint64 encoded in ASCII is 20 characters: "1844674407370955161" followed by "5"
		if first19 > 1844674407370955161 /* first 19 chars of largest uint64 */ || last > 5 {
			return 0, &strconv.NumError{Func: "decodeU64", Num: string(tokenBytes), Err: strconv.ErrRange}
		}
		return first19*10 + last, nil
	default: // must overflow
		return 0, &strconv.NumError{Func: "decodeU64", Num: string(tokenBytes), Err: strconv.ErrRange}
	}
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
