package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/wasm"
)

// EncodeTable returns the wasm.Table encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-table
func EncodeTable(i *wasm.Table) []byte {
	return append([]byte{i.Type}, EncodeLimitsType(i.Min, i.Max, false)...)
}
