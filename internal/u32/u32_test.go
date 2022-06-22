package u32

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		name  string
		input uint32
	}{
		{
			name:  "zero",
			input: 0,
		},
		{
			name:  "half",
			input: math.MaxInt32,
		},
		{
			name:  "max",
			input: math.MaxUint32,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			expected := make([]byte, 4)
			binary.LittleEndian.PutUint32(expected, tc.input)
			require.Equal(t, expected, LeBytes(tc.input))
		})
	}
}
