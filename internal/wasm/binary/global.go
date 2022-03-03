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
