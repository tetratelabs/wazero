package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestMemoryType(t *testing.T) {
	zero := uint32(0)
	max := wasm.MemoryMaxPages

	tests := []struct {
		name     string
		input    *wasm.Memory
		expected []byte
	}{
		{
			name:     "min 0",
			input:    &wasm.Memory{Max: wasm.MemoryMaxPages},
			expected: []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min 0, max 0",
			input:    &wasm.Memory{Max: zero},
			expected: []byte{0x1, 0, 0},
		},
		{
			name:     "min=max",
			input:    &wasm.Memory{Min: 1, Max: 1},
			expected: []byte{0x1, 1, 1},
		},
		{
			name:     "min 0, max largest",
			input:    &wasm.Memory{Max: max},
			expected: []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min largest max largest",
			input:    &wasm.Memory{Min: max, Max: max},
			expected: []byte{0x1, 0x80, 0x80, 0x4, 0x80, 0x80, 0x4},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeMemory(tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			binary, err := decodeMemory(bytes.NewReader(b), max)
			require.NoError(t, err)
			require.Equal(t, binary, tc.input)
		})
	}
}

func TestDecodeMemoryType_Errors(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		memoryMaxPages uint32
		expectedErr    string
	}{
		{
			name:        "max < min",
			input:       []byte{0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "min 65536 pages (4 Gi) > max 0 pages (0 Ki)",
		},
		{
			name:        "min > limit",
			input:       []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "min 4294967295 pages (3 Ti) outside range of 65536 pages (4 Gi)",
		},
		{
			name:        "max > limit",
			input:       []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "max 4294967295 pages (3 Ti) outside range of 65536 pages (4 Gi)",
		},
	}

	for _, tt := range tests {
		tc := tt

		if tc.memoryMaxPages == 0 {
			tc.memoryMaxPages = wasm.MemoryMaxPages
		}

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeMemory(bytes.NewReader(tc.input), tc.memoryMaxPages)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
