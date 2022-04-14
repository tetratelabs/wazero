package binary

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestLimitsType(t *testing.T) {
	zero := uint32(0)
	largest := uint32(math.MaxUint32)

	tests := []struct {
		name     string
		min      uint32
		max      *uint32
		expected []byte
	}{
		{
			name:     "min 0",
			expected: []byte{0x0, 0},
		},
		{
			name:     "min 0, max 0",
			max:      &zero,
			expected: []byte{0x1, 0, 0},
		},
		{
			name:     "min largest",
			min:      largest,
			expected: []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
		{
			name:     "min 0, max largest",
			max:      &largest,
			expected: []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
		{
			name:     "min largest max largest",
			min:      largest,
			max:      &largest,
			expected: []byte{0x1, 0xff, 0xff, 0xff, 0xff, 0xf, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeLimitsType(tc.min, tc.max)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			min, max, err := decodeLimitsType(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, min, tc.min)
			require.Equal(t, max, tc.max)
		})
	}
}
