package binaryencoding

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// EncodeImport returns the wasm.Import encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-import
func EncodeImport(i *wasm.Import) []byte {
	data := encodeSizePrefixed([]byte(i.Module))
	data = append(data, encodeSizePrefixed([]byte(i.Name))...)
	data = append(data, i.Type)
	switch i.Type {
	case wasm.ExternTypeFunc:
		data = append(data, leb128.EncodeUint32(i.DescFunc)...)
	case wasm.ExternTypeTable:
		data = append(data, wasm.RefTypeFuncref)
		data = append(data, EncodeLimitsType(i.DescTable.Min, i.DescTable.Max, false)...)
	case wasm.ExternTypeMemory:
		maxPtr := &i.DescMem.Max
		if !i.DescMem.IsMaxEncoded {
			maxPtr = nil
		}
		data = append(data, EncodeLimitsType(i.DescMem.Min, maxPtr, i.DescMem.IsShared)...)
	case wasm.ExternTypeGlobal:
		g := i.DescGlobal
		var mutable byte
		if g.Mutable {
			mutable = 1
		}
		data = append(data, g.ValType, mutable)
	default:
		panic(fmt.Errorf("invalid externtype: %s", wasm.ExternTypeName(i.Type)))
	}
	return data
}
