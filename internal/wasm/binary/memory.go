package binary

import (
	"bytes"
	"fmt"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

// decodeMemoryType returns the wasm.MemoryType decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func decodeMemoryType(r *bytes.Reader) (*wasm.MemoryType, error) {
	ret, err := decodeLimitsType(r)
	if err != nil {
		return nil, err
	}
	if ret.Min > wasm.MemoryMaxPages {
		return nil, fmt.Errorf("memory min must be at most 65536 pages (4GiB)")
	}
	if ret.Max != nil {
		if *ret.Max < ret.Min {
			return nil, fmt.Errorf("memory size minimum must not be greater than maximum")
		} else if *ret.Max > wasm.MemoryMaxPages {
			return nil, fmt.Errorf("memory max must be at most 65536 pages (4GiB)")
		}
	}
	return ret, nil
}

// encodeMemoryType returns the internalwasm.MemoryType encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func encodeMemoryType(i *wasm.MemoryType) []byte {
	return encodeLimitsType(i)
}
