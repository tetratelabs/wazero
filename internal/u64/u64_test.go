package u64

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
	}{
		{
			name:  "zero",
			input: 0,
		},
		{
			name:  "half",
			input: math.MaxUint32,
		},
		{
			name:  "max",
			input: math.MaxUint64,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			expected := make([]byte, 8)
			binary.LittleEndian.PutUint64(expected, tc.input)
			require.Equal(t, expected, LeBytes(tc.input))
		})
	}
}
