package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
)

// EncodeLimitsType returns the `limitsType` (min, max) encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#limits%E2%91%A6
//
// Extended in threads proposal: https://webassembly.github.io/threads/core/binary/types.html#limits
func EncodeLimitsType(min uint32, max *uint32, shared bool) []byte {
	var flag uint32
	if max != nil {
		flag = 0x01
	}
	if shared {
		flag |= 0x02
	}
	ret := append(leb128.EncodeUint32(flag), leb128.EncodeUint32(min)...)
	if max != nil {
		ret = append(ret, leb128.EncodeUint32(*max)...)
	}
	return ret
}
