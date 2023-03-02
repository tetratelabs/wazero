package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// encodeExport returns the wasm.Export encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#export-section%E2%91%A0
func encodeExport(i *wasm.Export) []byte {
	data := encodeSizePrefixed([]byte(i.Name))
	data = append(data, i.Type)
	data = append(data, leb128.EncodeUint32(i.Index)...)
	return data
}
