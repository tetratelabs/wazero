package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func decodeFunctionType(enabledFeatures api.CoreFeatures, r *bytes.Reader) (*wasm.FunctionType, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read leading byte: %w", err)
	}

	if b != 0x60 {
		return nil, fmt.Errorf("%w: %#x != 0x60", ErrInvalidByte, b)
	}

	paramCount, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("could not read parameter count: %w", err)
	}

	paramTypes, err := decodeValueTypes(r, paramCount)
	if err != nil {
		return nil, fmt.Errorf("could not read parameter types: %w", err)
	}

	resultCount, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("could not read result count: %w", err)
	}

	// Guard >1.0 feature multi-value
	if resultCount > 1 {
		if err = enabledFeatures.RequireEnabled(api.CoreFeatureMultiValue); err != nil {
			return nil, fmt.Errorf("multiple result types invalid as %v", err)
		}
	}

	resultTypes, err := decodeValueTypes(r, resultCount)
	if err != nil {
		return nil, fmt.Errorf("could not read result types: %w", err)
	}

	ret := &wasm.FunctionType{
		Params:  paramTypes,
		Results: resultTypes,
	}

	// cache the key for the function type
	_ = ret.String()

	return ret, nil
}
