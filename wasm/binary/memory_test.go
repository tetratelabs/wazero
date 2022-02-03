package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestMemoryType(t *testing.T) {
	zero := uint32(0)
	max := wasm.MemoryPageSize

	tests := []struct {
		name     string
		input    *wasm.MemoryType
		expected []byte
	}{
		{
			name:     "min 0",
			input:    &wasm.MemoryType{},
			expected: []byte{0x0, 0},
		},
		{
			name:     "min 0, max 0",
			input:    &wasm.MemoryType{Max: &zero},
			expected: []byte{0x1, 0, 0},
		},
		{
			name:     "min largest",
			input:    &wasm.MemoryType{Min: max},
			expected: []byte{0x0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min 0, max largest",
			input:    &wasm.MemoryType{Max: &max},
			expected: []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min largest max largest",
			input:    &wasm.MemoryType{Min: max, Max: &max},
			expected: []byte{0x1, 0x80, 0x80, 0x4, 0x80, 0x80, 0x4},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeMemoryType(tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			decoded, err := decodeMemoryType(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, decoded, tc.input)
		})
	}
}

func TestDecodeMemoryType_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "max < min",
			input:       []byte{0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "memory size minimum must not be greater than maximum",
		},
		{
			name:        "min > limit",
			input:       []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "memory min must be at most 65536 pages (4GiB)",
		},
		{
			name:        "max > limit",
			input:       []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "memory max must be at most 65536 pages (4GiB)",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeMemoryType(bytes.NewReader(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
