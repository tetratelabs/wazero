package internal

// decodeUint32 decodes an uint32 from a tokenUN or returns false on overflow
//
// Note: Bit length interpretation is not defined at the lexing layer, so this may fail on overflow due to invalid
// length known at the parsing layer.
//
// Note: This is similar to, but cannot use strconv.Atoi because WebAssembly allows underscore characters in numeric
// representation. See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#integers%E2%91%A6
func decodeUint32(tokenBytes []byte) (uint32, bool) { // TODO: hex
	// The max ASCII length of a uint32 is 10 (length of 4294967295). If we are at that length we can overflow.
	//
	// Note: There's chance we are at length 10 due to underscores, but easier to take the slow path for either reason.
	if len(tokenBytes) > 9 {
		v, overflow := decodeUint64(tokenBytes)
		if overflow {
			return 0, overflow
		}
		if v > 0xffffffff {
			return 0, true
		}
		return uint32(v), false
	}

	// If we are here, we know we cannot overflow uint32. The only valid characters in tokenUN are 0-9 and underscore.
	var n uint32
	for _, ch := range tokenBytes {
		if ch == '_' {
			continue
		}
		n = n*10 + uint32(ch-'0')
	}
	return n, false
}

// decodeUint64 is like decodeUint32, but for uint64
func decodeUint64(tokenBytes []byte) (uint64, bool) { // TODO: hex
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
	return n, false
}

// decodeUint64SlowPath implements decodeUint64 for the slow path when there's a chance the result cannot fit.
// Notably, this strips underscore characters first, so that the string length can be used to assess overflows.
func decodeUint64SlowPath(tokenBytes []byte) (uint64, bool) {
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
		first19, overflow := decodeUint64(tokenBytes[0:19])
		if overflow {
			return 0, false // impossible unless someone used this with an unvalidated token
		}
		last := uint64(tokenBytes[19] - '0')

		// Remember the largest uint64 encoded in ASCII is 20 characters: "1844674407370955161" followed by "5"
		if first19 > 1844674407370955161 /* first 19 chars of largest uint64 */ || last > 5 {
			return 0, true
		}
		return first19*10 + last, false
	default: // must overflow
		return 0, true
	}
}
