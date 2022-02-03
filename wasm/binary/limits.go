package binary

import (
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

// decodeLimitsType returns the wasm.LimitsType decoded with the WebAssembly 1.0 (MVP) Binary Format.
//
// See https://www.w3.org/TR/wasm-core-1/#limits%E2%91%A6
func decodeLimitsType(r io.Reader) (*wasm.LimitsType, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read leading byte: %v", err)
	}

	ret := &wasm.LimitsType{}
	switch b[0] {
	case 0x00:
		ret.Min, _, err = leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read min of limit: %v", err)
		}
	case 0x01:
		ret.Min, _, err = leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read min of limit: %v", err)
		}
		m, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read max of limit: %v", err)
		}
		ret.Max = &m
	default:
		return nil, fmt.Errorf("%v for limits: %#x != 0x00 or 0x01", ErrInvalidByte, b[0])
	}
	return ret, nil
}

// encodeLimitsType returns the wasm.LimitsType encoded in WebAssembly 1.0 (MVP) Binary Format.
//
// See https://www.w3.org/TR/wasm-core-1/#limits%E2%91%A6
func encodeLimitsType(l *wasm.LimitsType) []byte {
	if l.Max == nil {
		return append(leb128.EncodeUint32(0x00), leb128.EncodeUint32(l.Min)...)
	}
	return append(leb128.EncodeUint32(0x01), append(leb128.EncodeUint32(l.Min), leb128.EncodeUint32(*l.Max)...)...)
}
