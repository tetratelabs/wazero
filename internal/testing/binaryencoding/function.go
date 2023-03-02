package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/wasm"
)

// EncodeFunctionType returns the wasm.FunctionType encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// Note: Function types are encoded by the byte 0x60 followed by the respective vectors of parameter and result types.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-types%E2%91%A4
func EncodeFunctionType(t *wasm.FunctionType) []byte {
	// Only reached when "multi-value" is enabled because WebAssembly 1.0 (20191205) supports at most 1 result.
	data := append([]byte{0x60}, EncodeValTypes(t.Params)...)
	return append(data, EncodeValTypes(t.Results)...)
}
