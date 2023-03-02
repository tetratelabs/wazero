package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// encodeCode returns the wasm.Code encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-code
func encodeCode(c *wasm.Code) []byte {
	if c.GoFunc != nil {
		panic("BUG: GoFunction is not encodable")
	}

	// local blocks compress locals while preserving index order by grouping locals of the same type.
	// https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#code-section%E2%91%A0
	localBlockCount := uint32(0) // how many blocks of locals with the same type (types can repeat!)
	var localBlocks []byte
	localTypeLen := len(c.LocalTypes)
	if localTypeLen > 0 {
		i := localTypeLen - 1
		var runCount uint32              // count of the same type
		var lastValueType wasm.ValueType // initialize to an invalid type 0

		// iterate backwards so it is easier to size prefix
		for ; i >= 0; i-- {
			vt := c.LocalTypes[i]
			if lastValueType != vt {
				if runCount != 0 { // Only on the first iteration, this is zero when vt is compared against invalid
					localBlocks = append(leb128.EncodeUint32(runCount), localBlocks...)
				}
				lastValueType = vt
				localBlocks = append(leb128.EncodeUint32(uint32(vt)), localBlocks...) // reuse the EncodeUint32 cache
				localBlockCount++
				runCount = 1
			} else {
				runCount++
			}
		}
		localBlocks = append(leb128.EncodeUint32(runCount), localBlocks...)
		localBlocks = append(leb128.EncodeUint32(localBlockCount), localBlocks...)
	} else {
		localBlocks = leb128.EncodeUint32(0)
	}
	code := append(localBlocks, c.Body...)
	return append(leb128.EncodeUint32(uint32(len(code))), code...)
}
