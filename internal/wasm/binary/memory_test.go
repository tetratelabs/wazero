package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_newMemorySizer(t *testing.T) {
	zero := uint32(0)
	one := uint32(1)
	limit := wasm.MemoryLimitPages

	tests := []struct {
		name                                       string
		memoryCapacityFromMax                      bool
		min                                        uint32
		max                                        *uint32
		expectedMin, expectedCapacity, expectedMax uint32
	}{
		{
			name:             "min 0",
			min:              zero,
			max:              &limit,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      limit,
		},
		{
			name:             "min 0 defaults max to limit",
			min:              zero,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      limit,
		},
		{
			name:             "min 0, max 0",
			min:              zero,
			max:              &zero,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      zero,
		},
		{
			name:             "min 0, max 1",
			min:              zero,
			max:              &one,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      one,
		},
		{
			name:                  "min 0, max 1 memoryCapacityFromMax",
			memoryCapacityFromMax: true,
			min:                   zero,
			max:                   &one,
			expectedMin:           zero,
			expectedCapacity:      one,
			expectedMax:           one,
		},
		{
			name:             "min=max",
			min:              one,
			max:              &one,
			expectedMin:      one,
			expectedCapacity: one,
			expectedMax:      one,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			sizer := newMemorySizer(limit, tc.memoryCapacityFromMax)
			min, capacity, max := sizer(tc.min, tc.max)
			require.Equal(t, tc.expectedMin, min)
			require.Equal(t, tc.expectedCapacity, capacity)
			require.Equal(t, tc.expectedMax, max)
		})
	}
}

func TestMemoryType(t *testing.T) {
	zero := uint32(0)
	max := wasm.MemoryLimitPages

	tests := []struct {
		name     string
		input    *wasm.Memory
		expected []byte
	}{
		{
			name:     "min 0",
			input:    &wasm.Memory{Max: max, IsMaxEncoded: true},
			expected: []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min 0 default max",
			input:    &wasm.Memory{Max: max},
			expected: []byte{0x0, 0},
		},
		{
			name:     "min 0, max 0",
			input:    &wasm.Memory{Max: zero, IsMaxEncoded: true},
			expected: []byte{0x1, 0, 0},
		},
		{
			name:     "min=max",
			input:    &wasm.Memory{Min: 1, Cap: 1, Max: 1, IsMaxEncoded: true},
			expected: []byte{0x1, 1, 1},
		},
		{
			name:     "min 0, max largest",
			input:    &wasm.Memory{Max: max, IsMaxEncoded: true},
			expected: []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min largest max largest",
			input:    &wasm.Memory{Min: max, Cap: max, Max: max, IsMaxEncoded: true},
			expected: []byte{0x1, 0x80, 0x80, 0x4, 0x80, 0x80, 0x4},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeMemory(tc.input)
		t.Run(fmt.Sprintf("encode %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode %s", tc.name), func(t *testing.T) {
			binary, err := decodeMemory(bytes.NewReader(b), newMemorySizer(max, false), max)
			require.NoError(t, err)
			require.Equal(t, binary, tc.input)
		})
	}
}

func TestDecodeMemoryType_Errors(t *testing.T) {
	max := wasm.MemoryLimitPages

	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "max < min",
			input:       []byte{0x1, 0x80, 0x80, 0x4, 0},
			expectedErr: "min 65536 pages (4 Gi) > max 0 pages (0 Ki)",
		},
		{
			name:        "min > limit",
			input:       []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "min 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "max > limit",
			input:       []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "max 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeMemory(bytes.NewReader(tc.input), newMemorySizer(max, false), max)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
