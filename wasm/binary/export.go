package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

func decodeExport(r *bytes.Reader) (i *wasm.Export, err error) {
	i = &wasm.Export{}

	if i.Name, _, err = decodeUTF8(r, "export name"); err != nil {
		return nil, err
	}

	b := make([]byte, 1)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("error decoding export kind: %w", err)
	}

	i.Kind = b[0]
	switch i.Kind {
	case wasm.ExportKindFunc, wasm.ExportKindTable, wasm.ExportKindMemory, wasm.ExportKindGlobal:
		if i.Index, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding export index: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for exportdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}
