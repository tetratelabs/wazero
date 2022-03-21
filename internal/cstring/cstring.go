// Package cstring is named cstring because null-terminated strings are also known as CString and that avoids using
// clashing package names like "strings" or a really long one like "null-terminated-strings"
package cstring

import (
	"fmt"
	"unicode/utf8"
)

// NullTerminatedStrings holds null-terminated strings. It ensures that
// its length and total buffer size don't exceed the max of uint32.
// NullTerminatedStrings are convenience struct for args_get and environ_get. (environ_get is not implemented yet)
//
// A Null-terminated string is a byte string with a NULL suffix ("\x00").
// See https://en.wikipedia.org/wiki/Null-terminated_string
type NullTerminatedStrings struct {
	// NullTerminatedValues are null-terminated values with a NULL suffix.
	NullTerminatedValues [][]byte
	TotalBufSize         uint32
}

var EmptyNullTerminatedStrings = &NullTerminatedStrings{NullTerminatedValues: [][]byte{}}

// NewNullTerminatedStrings creates a NullTerminatedStrings from the given string slice. It returns an error
// if the length or the total buffer size of the result NullTerminatedStrings exceeds the maxBufSize
func NewNullTerminatedStrings(maxBufSize uint32, valName string, vals ...string) (*NullTerminatedStrings, error) {
	if len(vals) == 0 {
		return EmptyNullTerminatedStrings, nil
	}
	var strings [][]byte // don't pre-allocate as this function is size bound
	totalBufSize := uint32(0)
	for i, arg := range vals {
		if !utf8.ValidString(arg) {
			return nil, fmt.Errorf("%s[%d] is not a valid UTF-8 string", valName, i)
		}
		argLen := uint64(len(arg)) + 1 // + 1 for "\x00"; uint64 in case this one arg is huge
		nextSize := uint64(totalBufSize) + argLen
		if nextSize > uint64(maxBufSize) {
			return nil, fmt.Errorf("%s[%d] will exceed max buffer size %d", valName, i, maxBufSize)
		}
		totalBufSize = uint32(nextSize)
		strings = append(strings, append([]byte(arg), 0))
	}
	return &NullTerminatedStrings{NullTerminatedValues: strings, TotalBufSize: totalBufSize}, nil
}
