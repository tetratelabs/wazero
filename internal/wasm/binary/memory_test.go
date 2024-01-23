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

func Test_newMemorySizer(t *testing.T) {
	zero := uint32(0)
	ten := uint32(10)
	defaultLimit := wasm.MemoryLimitPages

	tests := []struct {
		name                                       string
		memoryCapacityFromMax                      bool
		limit                                      uint32
		min                                        uint32
		max                                        *uint32
		expectedMin, expectedCapacity, expectedMax uint32
	}{
		{
			name:             "min 0",
			limit:            defaultLimit,
			min:              zero,
			max:              &defaultLimit,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      defaultLimit,
		},
		{
			name:             "min 0 defaults max to defaultLimit",
			limit:            defaultLimit,
			min:              zero,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      defaultLimit,
		},
		{
			name:             "min 0, max 0",
			limit:            defaultLimit,
			min:              zero,
			max:              &zero,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      zero,
		},
		{
			name:             "min 0, max 10",
			limit:            defaultLimit,
			min:              zero,
			max:              &ten,
			expectedMin:      zero,
			expectedCapacity: zero,
			expectedMax:      ten,
		},
		{
			name:                  "min 0, max 10 memoryCapacityFromMax",
			limit:                 defaultLimit,
			memoryCapacityFromMax: true,
			min:                   zero,
			max:                   &ten,
			expectedMin:           zero,
			expectedCapacity:      ten,
			expectedMax:           ten,
		},
		{
			name:             "min 10, no max",
			limit:            200,
			min:              10,
			expectedMin:      10,
			expectedCapacity: 10,
			expectedMax:      200,
		},
		{
			name:                  "min 10, no max memoryCapacityFromMax",
			memoryCapacityFromMax: true,
			limit:                 200,
			min:                   10,
			expectedMin:           10,
			expectedCapacity:      200,
			expectedMax:           200,
		},
		{
			name:             "min=max",
			limit:            defaultLimit,
			min:              ten,
			max:              &ten,
			expectedMin:      ten,
			expectedCapacity: ten,
			expectedMax:      ten,
		},
		{
			name:             "max > memoryLimitPages",
			limit:            5,
			min:              0,
			max:              &ten,
			expectedMin:      0,
			expectedCapacity: 0,
			expectedMax:      5,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			sizer := newMemorySizer(tc.limit, tc.memoryCapacityFromMax)
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
		name             string
		input            *wasm.Memory
		memoryLimitPages uint32
		expected         []byte
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
		{
			name:             "min 0, max largest, wazero limit",
			input:            &wasm.Memory{Max: max, IsMaxEncoded: true},
			memoryLimitPages: 512,
			expected:         []byte{0x1, 0, 0x80, 0x80, 0x4},
		},
		{
			name:     "min 0, max 1, shared",
			input:    &wasm.Memory{Max: 1, IsMaxEncoded: true, IsShared: true},
			expected: []byte{0x3, 0, 1},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := binaryencoding.EncodeMemory(tc.input)
		t.Run(fmt.Sprintf("encode %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode %s", tc.name), func(t *testing.T) {
			tmax := max
			expectedDecoded := tc.input
			if tc.memoryLimitPages != 0 {
				// If a memory limit exists, then the expected module Max reflects that limit.
				tmax = tc.memoryLimitPages
				expectedDecoded.Max = tmax
			}

			features := api.CoreFeaturesV2
			if tc.input.IsShared {
				features = features.SetEnabled(experimental.CoreFeaturesThreads, true)
			}
			binary, err := decodeMemory(bytes.NewReader(b), features, newMemorySizer(tmax, false), tmax)
			require.NoError(t, err)
			require.Equal(t, binary, expectedDecoded)
		})
	}
}

func TestDecodeMemoryType_Errors(t *testing.T) {
	max := wasm.MemoryLimitPages

	tests := []struct {
		name           string
		input          []byte
		threadsEnabled bool
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
			expectedErr: "min 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "max > limit",
			input:       []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
			expectedErr: "max 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "shared but no threads",
			input:       []byte{0x2, 0, 0x80, 0x80, 0x4},
			expectedErr: "shared memory requested but threads feature not enabled",
		},
		{
			name:           "shared but no max",
			input:          []byte{0x2, 0, 0x80, 0x80, 0x4},
			threadsEnabled: true,
			expectedErr:    "shared memory requires a maximum size to be specified",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			features := api.CoreFeaturesV2
			if tc.threadsEnabled {
				features = features.SetEnabled(experimental.CoreFeaturesThreads, true)
			} else {
				// Allow test to work if threads is ever added to default features by explicitly removing threads features
				features = features.SetEnabled(experimental.CoreFeaturesThreads, false)
			}
			_, err := decodeMemory(bytes.NewReader(tc.input), features, newMemorySizer(max, false), max)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
