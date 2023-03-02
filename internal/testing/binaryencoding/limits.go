package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
)

// EncodeLimitsType returns the `limitsType` (min, max) encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#limits%E2%91%A6
func EncodeLimitsType(min uint32, max *uint32) []byte {
	if max == nil {
		return append(leb128.EncodeUint32(0x00), leb128.EncodeUint32(min)...)
	}
	return append(leb128.EncodeUint32(0x01), append(leb128.EncodeUint32(min), leb128.EncodeUint32(*max)...)...)
}
