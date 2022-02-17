package binary

import (
	"fmt"
	"io"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func decodeTableType(r io.Reader) (*wasm.TableType, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read leading byte: %v", err)
	}

	if b[0] != 0x70 {
		return nil, fmt.Errorf("%w: invalid element type %#x != %#x", ErrInvalidByte, b[0], 0x70)
	}

	lm, err := decodeLimitsType(r)
	if err != nil {
		return nil, fmt.Errorf("read limits: %v", err)
	}

	return &wasm.TableType{
		ElemType: 0x70, // funcref
		Limit:    lm,
	}, nil
}

func decodeGlobalType(r io.Reader) (*wasm.GlobalType, error) {
	vt, err := decodeValueTypes(r, 1)
	if err != nil {
		return nil, fmt.Errorf("read value type: %w", err)
	}

	ret := &wasm.GlobalType{
		ValType: vt[0],
	}

	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read mutablity: %w", err)
	}

	switch mut := b[0]; mut {
	case 0x00:
	case 0x01:
		ret.Mutable = true
	default:
		return nil, fmt.Errorf("%w for mutability: %#x != 0x00 or 0x01", ErrInvalidByte, mut)
	}
	return ret, nil
}

var nullary = []byte{0x60, 0, 0}

// encodedOneParam is a cache of FunctionType.encode values for param length 1 and result length 0
var encodedOneParam = map[wasm.ValueType][]byte{
	wasm.ValueTypeI32: {0x60, 1, wasm.ValueTypeI32, 0},
	wasm.ValueTypeI64: {0x60, 1, wasm.ValueTypeI64, 0},
	wasm.ValueTypeF32: {0x60, 1, wasm.ValueTypeF32, 0},
	wasm.ValueTypeF64: {0x60, 1, wasm.ValueTypeF64, 0},
}

// encodedOneResult is a cache of FunctionType.encode values for param length 0 and result length 1
var encodedOneResult = map[wasm.ValueType][]byte{
	wasm.ValueTypeI32: {0x60, 0, 1, wasm.ValueTypeI32},
	wasm.ValueTypeI64: {0x60, 0, 1, wasm.ValueTypeI64},
	wasm.ValueTypeF32: {0x60, 0, 1, wasm.ValueTypeF32},
	wasm.ValueTypeF64: {0x60, 0, 1, wasm.ValueTypeF64},
}

// encodeFunctionType returns the wasm.FunctionType encoded in WebAssembly 1.0 (MVP) Binary Format.
//
// Note: Function types are encoded by the byte 0x60 followed by the respective vectors of parameter and result types.
// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A4
func encodeFunctionType(t *wasm.FunctionType) []byte {
	paramCount, resultCount := len(t.Params), len(t.Results)
	if paramCount == 0 && resultCount == 0 {
		return nullary
	}
	if resultCount == 0 {
		if paramCount == 1 {
			if encoded, ok := encodedOneParam[t.Params[0]]; ok {
				return encoded
			}
		}
		return append(append([]byte{0x60}, encodeValTypes(t.Params)...), 0)
	} else if resultCount == 1 {
		if paramCount == 0 {
			if encoded, ok := encodedOneResult[t.Results[0]]; ok {
				return encoded
			}
		}
		return append(append([]byte{0x60}, encodeValTypes(t.Params)...), 1, t.Results[0])
	}
	// This branch should never be reaches as WebAssembly 1.0 (MVP) supports at most 1 result
	data := append([]byte{0x60}, encodeValTypes(t.Params)...)
	return append(data, encodeValTypes(t.Results)...)
}
