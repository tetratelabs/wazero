package binary

import (
	"bytes"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func decodeGlobal(r *bytes.Reader, features wasm.Features) (*wasm.Global, error) {
	gt, err := decodeGlobalType(r, features)
	if err != nil {
		return nil, err
	}

	init, err := decodeConstantExpression(r)
	if err != nil {
		return nil, err
	}

	return &wasm.Global{Type: gt, Init: init}, nil
}

// encodeGlobal returns the internalwasm.Global encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-section%E2%91%A0
func encodeGlobal(g *wasm.Global) []byte {
	var mutable byte
	if g.Type.Mutable {
		mutable = 1
	}
	data := []byte{g.Type.ValType, mutable, g.Init.Opcode}
	data = append(data, g.Init.Data...)
	return append(data, wasm.OpcodeEnd)
}
