package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// decodeTable returns the wasm.Table decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-table
func decodeTable(r *bytes.Reader) (*wasm.Table, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read leading byte: %v", err)
	}

	if b != wasm.ElemTypeFuncref {
		return nil, fmt.Errorf("invalid element type %#x != funcref(%#x)", b, wasm.ElemTypeFuncref)
	}

	min, max, err := decodeLimitsType(r)
	if err != nil {
		return nil, fmt.Errorf("read limits: %v", err)
	}
	if min > wasm.MaximumFunctionIndex {
		return nil, fmt.Errorf("table min must be at most %d", wasm.MaximumFunctionIndex)
	}
	if max != nil {
		if *max < min {
			return nil, fmt.Errorf("table size minimum must not be greater than maximum")
		} else if *max > wasm.MaximumFunctionIndex {
			return nil, fmt.Errorf("table max must be at most %d", wasm.MaximumFunctionIndex)
		}
	}
	return &wasm.Table{Min: min, Max: max}, nil
}

// encodeTable returns the wasm.Table encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-table
func encodeTable(i *wasm.Table) []byte {
	return append([]byte{wasm.ElemTypeFuncref}, encodeLimitsType(i.Min, i.Max)...)
}
