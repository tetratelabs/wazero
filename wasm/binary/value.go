package binary

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

var noValType = []byte{0}

// encodedValTypes is a cache of size prefixed binary encoding of known val types.
var encodedValTypes = map[wasm.ValueType][]byte{
	wasm.ValueTypeI32: {1, wasm.ValueTypeI32},
	wasm.ValueTypeI64: {1, wasm.ValueTypeI64},
	wasm.ValueTypeF32: {1, wasm.ValueTypeF32},
	wasm.ValueTypeF64: {1, wasm.ValueTypeF64},
}

// encodeValTypes fast paths binary encoding of common value type lengths
func encodeValTypes(vt []wasm.ValueType) []byte {
	// Special case nullary and parameter lengths of wasi_snapshot_preview1 to avoid excess allocations
	switch uint32(len(vt)) {
	case 0: // nullary
		return noValType
	case 1: // ex $wasi_snapshot_preview1.fd_close or any result
		if encoded, ok := encodedValTypes[vt[0]]; ok {
			return encoded
		}
	case 2: // ex $wasi_snapshot_preview1.environ_sizes_get
		return []byte{2, vt[0], vt[1]}
	case 4: // ex $wasi_snapshot_preview1.fd_write
		return []byte{4, vt[0], vt[1], vt[2], vt[3]}
	case 9: // ex $wasi_snapshot_preview1.fd_write
		return []byte{9, vt[0], vt[1], vt[2], vt[3], vt[4], vt[5], vt[6], vt[7], vt[8]}
	}
	// Slow path others until someone complains with a valid signature
	count := leb128.EncodeUint32(uint32(len(vt)))
	return append(count, vt...)
}

func decodeValueTypes(r io.Reader, num uint32) ([]wasm.ValueType, error) {
	ret := make([]wasm.ValueType, num)
	buf := make([]wasm.ValueType, num)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	for i, v := range buf {
		switch v {
		case wasm.ValueTypeI32, wasm.ValueTypeF32, wasm.ValueTypeI64, wasm.ValueTypeF64:
			ret[i] = v
		default:
			return nil, fmt.Errorf("invalid value type: %d", v)
		}
	}
	return ret, nil
}

func decodeNameValue(r io.Reader) (string, error) {
	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return "", fmt.Errorf("read size of name: %v", err)
	}

	buf := make([]byte, vs)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read bytes of name: %v", err)
	}

	if !utf8.Valid(buf) {
		return "", fmt.Errorf("name must be valid as utf8")
	}

	return string(buf), nil
}
