package wasi

import (
	"fmt"
	"unicode/utf8"
)

// nullTerminatedStrings holds null-terminated strings. It ensures that
// its length and total buffer size don't exceed the max of uint32.
// nullTerminatedStrings are convenience struct for args_get and environ_get. (environ_get is not implemented yet)
//
// A Null-terminated string is a byte string with a NULL suffix ("\x00").
// See https://en.wikipedia.org/wiki/Null-terminated_string
type nullTerminatedStrings struct {
	// nullTerminatedValues are null-terminated values with a NULL suffix.
	nullTerminatedValues [][]byte
	totalBufSize         uint32
}

// newNullTerminatedStrings creates a nullTerminatedStrings from the given string slice. It returns an error
// if the length or the total buffer size of the result WASIStringArray exceeds the maxBufSize
func newNullTerminatedStrings(maxBufSize uint32, args ...string) (*nullTerminatedStrings, error) {
	if len(args) == 0 {
		return &nullTerminatedStrings{nullTerminatedValues: [][]byte{}}, nil
	}
	var strings [][]byte // don't pre-allocate as this function is size bound
	totalBufSize := uint32(0)
	for i, arg := range args {
		if !utf8.ValidString(arg) {
			return nil, fmt.Errorf("arg[%d] is not a valid UTF-8 string", i)
		}
		argLen := uint64(len(arg)) + 1 // + 1 for "\x00"; uint64 in case this one arg is huge
		nextSize := uint64(totalBufSize) + argLen
		if nextSize > uint64(maxBufSize) {
			return nil, fmt.Errorf("arg[%d] will exceed max buffer size %d", i, maxBufSize)
		}
		totalBufSize = uint32(nextSize)
		strings = append(strings, append([]byte(arg), 0))
	}
	return &nullTerminatedStrings{nullTerminatedValues: strings, totalBufSize: totalBufSize}, nil
}
