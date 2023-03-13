package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/wasm"
)

// encodeGlobal returns the wasm.Global encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-section%E2%91%A0
func encodeGlobal(g wasm.Global) (data []byte) {
	var mutable byte
	if g.Type.Mutable {
		mutable = 1
	}
	data = []byte{g.Type.ValType, mutable}
	data = append(data, encodeConstantExpression(g.Init)...)
	return
}
