package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// decodeMemory returns the api.Memory decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func decodeMemory(r *bytes.Reader, memoryMaxPages uint32) (*wasm.Memory, error) {
	min, maxP, err := decodeLimitsType(r)
	if err != nil {
		return nil, err
	}
	var max uint32
	if maxP != nil {
		max = *maxP
	} else {
		max = memoryMaxPages
	}
	if max > memoryMaxPages {
		return nil, fmt.Errorf("max %d pages (%s) outside range of %d pages (%s)", max, wasm.PagesToUnitOfBytes(max), memoryMaxPages, wasm.PagesToUnitOfBytes(memoryMaxPages))
	} else if min > memoryMaxPages {
		return nil, fmt.Errorf("min %d pages (%s) outside range of %d pages (%s)", min, wasm.PagesToUnitOfBytes(min), memoryMaxPages, wasm.PagesToUnitOfBytes(memoryMaxPages))
	} else if min > max {
		return nil, fmt.Errorf("min %d pages (%s) > max %d pages (%s)", min, wasm.PagesToUnitOfBytes(min), max, wasm.PagesToUnitOfBytes(max))
	}
	return &wasm.Memory{Min: min, Max: max}, nil
}

// encodeMemory returns the wasm.Memory encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func encodeMemory(i *wasm.Memory) []byte {
	return encodeLimitsType(i.Min, &i.Max)
}
