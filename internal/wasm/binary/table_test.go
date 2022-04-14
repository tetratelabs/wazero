package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/wasm"
)

func TestTableType(t *testing.T) {
	zero := uint32(0)
	max := wasm.MaximumFunctionIndex

	tests := []struct {
		name     string
		input    *wasm.Table
		expected []byte
	}{
		{
			name:     "min 0",
			input:    &wasm.Table{},
			expected: []byte{wasm.ElemTypeFuncref, 0x0, 0},
		},
		{
			name:     "min 0, max 0",
			input:    &wasm.Table{Max: &zero},
			expected: []byte{wasm.ElemTypeFuncref, 0x1, 0, 0},
		},
		{
			name:     "min largest",
			input:    &wasm.Table{Min: max},
			expected: []byte{wasm.ElemTypeFuncref, 0x0, 0x80, 0x80, 0x80, 0x40},
		},
		{
			name:     "min 0, max largest",
			input:    &wasm.Table{Max: &max},
			expected: []byte{wasm.ElemTypeFuncref, 0x1, 0, 0x80, 0x80, 0x80, 0x40},
		},
		{
			name:     "min largest max largest",
			input:    &wasm.Table{Min: max, Max: &max},
			expected: []byte{wasm.ElemTypeFuncref, 0x1, 0x80, 0x80, 0x80, 0x40, 0x80, 0x80, 0x80, 0x40},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeTable(tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			decoded, err := decodeTable(bytes.NewReader(b))
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
	}{
		{
			name:        "not func ref",
			input:       []byte{0x50, 0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "invalid element type 0x50 != funcref(0x70)",
		},
		{
			name:        "max < min",
			input:       []byte{wasm.ElemTypeFuncref, 0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "table size minimum must not be greater than maximum",
		},
		{
			name:        "min > limit",
			input:       []byte{wasm.ElemTypeFuncref, 0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "table min must be at most 134217728",
		},
		{
			name:        "max > limit",
			input:       []byte{wasm.ElemTypeFuncref, 0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "table max must be at most 134217728",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeTable(bytes.NewReader(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
