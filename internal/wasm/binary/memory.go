package binary

import (
	"bytes"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// decodeMemory returns the api.Memory decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func decodeMemory(r *bytes.Reader, memoryLimitPages uint32) (*wasm.Memory, error) {
	min, maxP, err := decodeLimitsType(r)
	if err != nil {
		return nil, err
	}

	var max uint32
	var isMaxEncoded bool
	if maxP != nil {
		isMaxEncoded = true
		max = *maxP
	}
	mem := &wasm.Memory{Min: min, Max: max, IsMaxEncoded: isMaxEncoded}
	return mem, mem.ValidateMinMax(memoryLimitPages)
}

// encodeMemory returns the wasm.Memory encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-memory
func encodeMemory(i *wasm.Memory) []byte {
	maxPtr := &i.Max
	if !i.IsMaxEncoded {
		maxPtr = nil
	}
	return encodeLimitsType(i.Min, maxPtr)
}
