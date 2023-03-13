package binary

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// decodeTable returns the wasm.Table decoded with the WebAssembly 1.0 (20191205) Binary Format.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-table
func decodeTable(r *bytes.Reader, enabledFeatures api.CoreFeatures) (wasm.Table, error) {
	tableType, err := r.ReadByte()
	if err != nil {
		return wasm.Table{}, fmt.Errorf("read leading byte: %v", err)
	}

	if tableType != wasm.RefTypeFuncref {
		if err := enabledFeatures.RequireEnabled(api.CoreFeatureReferenceTypes); err != nil {
			return wasm.Table{}, fmt.Errorf("table type funcref is invalid: %w", err)
		}
	}

	min, max, err := decodeLimitsType(r)
	if err != nil {
		return wasm.Table{}, fmt.Errorf("read limits: %v", err)
	}
	if min > wasm.MaximumFunctionIndex {
		return wasm.Table{}, fmt.Errorf("table min must be at most %d", wasm.MaximumFunctionIndex)
	}
	if max != nil {
		if *max < min {
			return wasm.Table{}, fmt.Errorf("table size minimum must not be greater than maximum")
		}
	}
	return wasm.Table{Min: min, Max: max, Type: tableType}, nil
}
