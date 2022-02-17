package binary

import (
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/internal/leb128"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	wasm2 "github.com/tetratelabs/wazero/wasm"
)

func decodeCode(r io.Reader) (*wasm.Code, error) {
	ss, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size of code: %w", err)
	}

	r = io.LimitReader(r, int64(ss))

	// parse locals
	ls, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size locals: %v", err)
	}

	var nums []uint64
	var types []wasm2.ValueType
	var sum uint64
	b := make([]byte, 1)
	for i := uint32(0); i < ls; i++ {
		n, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read n of locals: %v", err)
		}
		sum += uint64(n)
		nums = append(nums, uint64(n))

		_, err = io.ReadFull(r, b)
		if err != nil {
			return nil, fmt.Errorf("read type of local: %v", err)
		}
		switch vt := b[0]; vt {
		case wasm2.ValueTypeI32, wasm2.ValueTypeF32, wasm2.ValueTypeI64, wasm2.ValueTypeF64:
			types = append(types, vt)
		default:
			return nil, fmt.Errorf("invalid local type: 0x%x", vt)
		}
	}

	if sum > math.MaxUint32 {
		return nil, fmt.Errorf("too many locals: %d", sum)
	}

	var localTypes []wasm2.ValueType
	for i, num := range nums {
		t := types[i]
		for j := uint64(0); j < num; j++ {
			localTypes = append(localTypes, t)
		}
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if body[len(body)-1] != wasm.OpcodeEnd {
		return nil, fmt.Errorf("expr not end with OpcodeEnd")
	}

	return &wasm.Code{Body: body, LocalTypes: localTypes}, nil
}

// encodeCode returns the wasm.Code encoded in WebAssembly 1.0 (MVP) Binary Format.
//
// See https://www.w3.org/TR/wasm-core-1/#binary-code
func encodeCode(c *wasm.Code) []byte {
	// local blocks compress locals while preserving index order by grouping locals of the same type.
	// https://www.w3.org/TR/wasm-core-1/#code-section%E2%91%A0
	localBlockCount := uint32(0) // how many blocks of locals with the same type (types can repeat!)
	var localBlocks []byte
	localTypeLen := len(c.LocalTypes)
	if localTypeLen > 0 {
		i := localTypeLen - 1
		var runCount uint32               // count of the same type
		var lastValueType wasm2.ValueType // initialize to an invalid type 0

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
