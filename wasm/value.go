package wasm

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

// ValueType is the binary encoding of a type such as i32
// See https://www.w3.org/TR/wasm-core-1/#binary-valtype
//
// Note: This is a type alias as it is easier to encode and decode in the binary format.
type ValueType = byte

const (
	ValueTypeI32 ValueType = 0x7f
	ValueTypeI64 ValueType = 0x7e
	ValueTypeF32 ValueType = 0x7d
	ValueTypeF64 ValueType = 0x7c
)

func readValueTypes(r io.Reader, num uint32) ([]ValueType, error) {
	ret := make([]ValueType, num)
	buf := make([]ValueType, num)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	for i, v := range buf {
		switch v {
		case ValueTypeI32, ValueTypeF32, ValueTypeI64, ValueTypeF64:
			ret[i] = v
		default:
			return nil, fmt.Errorf("invalid value type: %d", v)
		}
	}
	return ret, nil
}

func readNameValue(r io.Reader) (string, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return "", fmt.Errorf("read size of name: %v", err)
	}

	buf := make([]byte, vs)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read bytes of name: %v", err)
	}

	if !utf8.Valid(buf) {
		return "", fmt.Errorf("name must be valid as utf8")
	}

	return string(buf), nil
}

func HasSameSignature(a []ValueType, b []ValueType) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
