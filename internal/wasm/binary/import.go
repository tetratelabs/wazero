package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/internal/leb128"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func decodeImport(r *bytes.Reader) (i *wasm.Import, err error) {
	i = &wasm.Import{}
	if i.Module, _, err = decodeUTF8(r, "import module"); err != nil {
		return nil, err
	}

	if i.Name, _, err = decodeUTF8(r, "import name"); err != nil {
		return nil, err
	}

	b := make([]byte, 1)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("error decoding import kind: %w", err)
	}

	i.Kind = b[0]
	switch i.Kind {
	case wasm.ExternalKindFunc:
		if i.DescFunc, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding import func typeindex: %w", err)
		}
	case wasm.ExternalKindTable:
		if i.DescTable, err = decodeTableType(r); err != nil {
			return nil, fmt.Errorf("error decoding import table desc: %w", err)
		}
	case wasm.ExternalKindMemory:
		if i.DescMem, err = decodeMemoryType(r); err != nil {
			return nil, fmt.Errorf("error decoding import mem desc: %w", err)
		}
	case wasm.ExternalKindGlobal:
		if i.DescGlobal, err = decodeGlobalType(r); err != nil {
			return nil, fmt.Errorf("error decoding import global desc: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for importdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}

// encodeImport returns the wasm.Import encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-import
func encodeImport(i *wasm.Import) []byte {
	data := encodeSizePrefixed([]byte(i.Module))
	data = append(data, encodeSizePrefixed([]byte(i.Name))...)
	data = append(data, i.Kind)
	switch i.Kind {
	case wasm.ExternalKindFunc:
		data = append(data, leb128.EncodeUint32(i.DescFunc)...)
	case wasm.ExternalKindTable:
		panic("TODO: encodeExternalKindTable")
	case wasm.ExternalKindMemory:
		panic("TODO: encodeExternalKindMemory")
	case wasm.ExternalKindGlobal:
		panic("TODO: encodeExternalKindGlobal")
	default:
		panic(fmt.Errorf("invalid kind: %#x", i.Kind))
	}
	return data
}
