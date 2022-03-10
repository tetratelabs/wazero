package binary

import (
	"bytes"
	"fmt"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

// decodeMemory returns the wasm.Memory decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func decodeMemory(r *bytes.Reader) (*wasm.Memory, error) {
	min, max, err := decodeLimitsType(r)
	if err != nil {
		return nil, err
	}
	if min > wasm.MemoryMaxPages {
		return nil, fmt.Errorf("memory min must be at most 65536 pages (4GiB)")
	}
	if max != nil {
		if *max < min {
			return nil, fmt.Errorf("memory size minimum must not be greater than maximum")
		} else if *max > wasm.MemoryMaxPages {
			return nil, fmt.Errorf("memory max must be at most 65536 pages (4GiB)")
		}
	}
	return &wasm.Memory{Min: min, Max: max}, nil
}

// encodeMemory returns the internalwasm.Memory encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func encodeMemory(i *wasm.Memory) []byte {
	return encodeLimitsType(i.Min, i.Max)
}
