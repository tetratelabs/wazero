package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var noValType = []byte{0}

// encodedValTypes is a cache of size prefixed binary encoding of known val types.
var encodedValTypes = map[wasm.ValueType][]byte{
	wasm.ValueTypeI32:       {1, wasm.ValueTypeI32},
	wasm.ValueTypeI64:       {1, wasm.ValueTypeI64},
	wasm.ValueTypeF32:       {1, wasm.ValueTypeF32},
	wasm.ValueTypeF64:       {1, wasm.ValueTypeF64},
	wasm.ValueTypeExternref: {1, wasm.ValueTypeExternref},
	wasm.ValueTypeFuncref:   {1, wasm.ValueTypeFuncref},
	wasm.ValueTypeV128:      {1, wasm.ValueTypeV128},
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
	case 2: // ex $wasi.environ_sizes_get
		return []byte{2, vt[0], vt[1]}
	case 4: // ex $wasi.fd_write
		return []byte{4, vt[0], vt[1], vt[2], vt[3]}
	case 9: // ex $wasi.fd_write
		return []byte{9, vt[0], vt[1], vt[2], vt[3], vt[4], vt[5], vt[6], vt[7], vt[8]}
	}
	// Slow path others until someone complains with a valid signature
	count := leb128.EncodeUint32(uint32(len(vt)))
	return append(count, vt...)
}
