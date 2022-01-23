package binary

import (
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
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
	var types []wasm.ValueType
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
		switch vt := wasm.ValueType(b[0]); vt {
		case wasm.ValueTypeI32, wasm.ValueTypeF32, wasm.ValueTypeI64, wasm.ValueTypeF64:
			types = append(types, vt)
		default:
			return nil, fmt.Errorf("invalid local type: 0x%x", vt)
		}
	}

	if sum > math.MaxUint32 {
		return nil, fmt.Errorf("too many locals: %d", sum)
	}

	var localTypes []wasm.ValueType
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

	if body[len(body)-1] != byte(wasm.OpcodeEnd) {
		return nil, fmt.Errorf("expr not end with OpcodeEnd")
	}

	return &wasm.Code{
		Body:       body,
		NumLocals:  uint32(sum),
		LocalTypes: localTypes,
	}, nil
}
