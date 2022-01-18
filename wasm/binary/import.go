package binary

import (
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

func decodeImport(r io.Reader) (i *wasm.Import, err error) {
	i = &wasm.Import{}
	if i.Module, err = decodeNameValue(r); err != nil {
		return nil, fmt.Errorf("error decoding import module: %w", err)
	}

	if i.Name, err = decodeNameValue(r); err != nil {
		return nil, fmt.Errorf("error decoding import name: %w", err)
	}

	b := make([]byte, 1)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("error decoding import kind: %w", err)
	}

	i.Kind = b[0]
	switch i.Kind {
	case wasm.ImportKindFunc:
		if i.DescFunc, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding import func typeindex: %w", err)
		}
	case wasm.ImportKindTable:
		if i.DescTable, err = decodeTableType(r); err != nil {
			return nil, fmt.Errorf("error decoding import table desc: %w", err)
		}
	case wasm.ImportKindMemory:
		if i.DescMem, err = decodeMemoryType(r); err != nil {
			return nil, fmt.Errorf("error decoding import mem desc: %w", err)
		}
	case wasm.ImportKindGlobal:
		if i.DescGlobal, err = decodeGlobalType(r); err != nil {
			return nil, fmt.Errorf("error decoding import global desc: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for importdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}
