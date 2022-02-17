package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/internal/leb128"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	wasm2 "github.com/tetratelabs/wazero/wasm"
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
	case wasm2.ExportKindFunc, wasm2.ExportKindTable, wasm2.ExportKindMemory, wasm2.ExportKindGlobal:
		if i.Index, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding export index: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for exportdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}

// encodeExport returns the wasm.Export encoded in WebAssembly 1.0 (MVP) Binary Format.
//
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
func encodeExport(i *wasm.Export) []byte {
	data := encodeSizePrefixed([]byte(i.Name))
	data = append(data, i.Kind)
	data = append(data, leb128.EncodeUint32(i.Index)...)
	return data
}
