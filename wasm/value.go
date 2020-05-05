package wasm

import (
	"fmt"
	"io"

	"github.com/mathetake/gasm/wasm/leb128"
)

type ValueType byte

const (
	ValueTypeI32 ValueType = 0x7f
	ValueTypeI64 ValueType = 0x7e
	ValueTypeF32 ValueType = 0x7d
	ValueTypeF64 ValueType = 0x7c
)

func readValueTypes(r io.Reader, num uint32) ([]ValueType, error) {
	ret := make([]ValueType, num)
	buf := make([]byte, num)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	for i, v := range buf {
		switch vt := ValueType(v); vt {
		case ValueTypeI32, ValueTypeF32, ValueTypeI64, ValueTypeF64:
			ret[i] = vt
		default:
			return nil, fmt.Errorf("invalid value type: %d", vt)
		}
	}
	return ret, nil
}

func readNameValue(r io.Reader) (string, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return "", fmt.Errorf("read size of name: %w", err)
	}

	buf := make([]byte, vs)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read bytes of name: %w", err)
	}

	return string(buf), nil
}

func hasSameSignature(a []ValueType, b []ValueType) bool {
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
