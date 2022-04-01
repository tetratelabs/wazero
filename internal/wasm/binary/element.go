package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeElementSegment(r *bytes.Reader) (*wasm.ElementSegment, error) {
	ti, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get table index: %w", err)
	}
	if ti > 0 {
		return nil, fmt.Errorf("at most one table allowed in module, but read index %d", ti)
	}

	expr, err := decodeConstantExpression(r)
	if err != nil {
		return nil, fmt.Errorf("read expr for offset: %w", err)
	}

	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	init := make([]uint32, vs)
	for i := range init {
		fIDx, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read function index: %w", err)
		}
		init[i] = fIDx
	}

	return &wasm.ElementSegment{OffsetExpr: expr, Init: init}, nil
}
