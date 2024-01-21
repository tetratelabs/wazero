package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestTableType(t *testing.T) {
	zero := uint32(0)
	max := wasm.MaximumFunctionIndex

	tests := []struct {
		name     string
		input    wasm.Table
		expected []byte
	}{
		{
			name:     "min 0 - funcref",
			input:    wasm.Table{Type: wasm.RefTypeFuncref},
			expected: []byte{wasm.RefTypeFuncref, 0x0, 0},
		},
		{
			name:     "min 0 - externref",
			input:    wasm.Table{Type: wasm.RefTypeExternref},
			expected: []byte{wasm.RefTypeExternref, 0x0, 0},
		},
		{
			name:     "min 0, max 0",
			input:    wasm.Table{Max: &zero, Type: wasm.RefTypeFuncref},
			expected: []byte{wasm.RefTypeFuncref, 0x1, 0, 0},
		},
		{
			name:     "min largest",
			input:    wasm.Table{Min: max, Type: wasm.RefTypeFuncref},
			expected: []byte{wasm.RefTypeFuncref, 0x0, 0x80, 0x80, 0x80, 0x40},
		},
		{
			name:     "min 0, max largest",
			input:    wasm.Table{Max: &max, Type: wasm.RefTypeFuncref},
			expected: []byte{wasm.RefTypeFuncref, 0x1, 0, 0x80, 0x80, 0x80, 0x40},
		},
		{
			name:     "min largest max largest",
			input:    wasm.Table{Min: max, Max: &max, Type: wasm.RefTypeFuncref},
			expected: []byte{wasm.RefTypeFuncref, 0x1, 0x80, 0x80, 0x80, 0x40, 0x80, 0x80, 0x80, 0x40},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := binaryencoding.EncodeTable(&tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			var decoded wasm.Table
			err := decodeTable(bytes.NewReader(b), api.CoreFeatureReferenceTypes, &decoded)
			require.NoError(t, err)
			require.Equal(t, decoded, tc.input)
		})
	}
}

func TestDecodeTableType_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
		features    api.CoreFeatures
	}{
		{
			name:        "not func ref",
			input:       []byte{0x50, 0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "table type funcref is invalid: feature \"reference-types\" is disabled",
		},
		{
			name:        "max < min",
			input:       []byte{wasm.RefTypeFuncref, 0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "table size minimum must not be greater than maximum",
			features:    api.CoreFeatureReferenceTypes,
		},
		{
			name:        "min > limit",
			input:       []byte{wasm.RefTypeFuncref, 0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "table min must be at most 134217728",
			features:    api.CoreFeatureReferenceTypes,
		},
		{
			name:        "shared",
			input:       []byte{wasm.RefTypeFuncref, 0x2, 0},
			expectedErr: "tables cannot be marked as shared",
			// Shared tables are an error even if threads are enabled.
			features: experimental.CoreFeaturesThreads,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			var decoded wasm.Table
			err := decodeTable(bytes.NewReader(tc.input), tc.features, &decoded)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
