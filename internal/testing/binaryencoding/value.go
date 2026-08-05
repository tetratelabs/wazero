package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var noValType = []byte{0}

// encodedValTypes is a cache of size prefixed binary encoding of known val types.
var encodedValTypes = map[wasm.ValueType][]byte{
	wasm.ValueTypeI32:       {1, wasm.ValueTypeI32.Kind()},
	wasm.ValueTypeI64:       {1, wasm.ValueTypeI64.Kind()},
	wasm.ValueTypeF32:       {1, wasm.ValueTypeF32.Kind()},
	wasm.ValueTypeF64:       {1, wasm.ValueTypeF64.Kind()},
	wasm.ValueTypeExternref: {1, wasm.ValueTypeExternref.Kind()},
	wasm.ValueTypeFuncref:   {1, wasm.ValueTypeFuncref.Kind()},
	wasm.ValueTypeV128:      {1, wasm.ValueTypeV128.Kind()},
}

// EncodeValTypes fast paths binary encoding of common value type lengths
func EncodeValTypes(vt []wasm.ValueType) []byte {
	// Special case nullary and parameter lengths of wasi_snapshot_preview1 to avoid excess allocations
	switch uint32(len(vt)) {
	case 0: // nullary
		return noValType
	case 1: // ex $wasi.fd_close or any result
		if encoded, ok := encodedValTypes[vt[0]]; ok {
			return encoded
		}
	}
	// Encode value types as their Kind bytes
	count := leb128.EncodeUint32(uint32(len(vt)))
	for _, v := range vt {
		count = append(count, v.Kind())
	}
	return count
}
