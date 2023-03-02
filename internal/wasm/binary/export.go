package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeExport(r *bytes.Reader) (i *wasm.Export, err error) {
	i = &wasm.Export{}

	if i.Name, _, err = decodeUTF8(r, "export name"); err != nil {
		return nil, err
	}

	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("error decoding export kind: %w", err)
	}

	i.Type = b
	switch i.Type {
	case wasm.ExternTypeFunc, wasm.ExternTypeTable, wasm.ExternTypeMemory, wasm.ExternTypeGlobal:
		if i.Index, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding export index: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for exportdesc: %#x", ErrInvalidByte, b)
	}
	return
}
