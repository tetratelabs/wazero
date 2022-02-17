package binary

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func TestLimitsType(t *testing.T) {
	zero := uint32(0)
	max := uint32(math.MaxUint32)

	tests := []struct {
		name     string
		input    *wasm.LimitsType
		expected []byte
	}{
		{
			name:     "min 0",
			input:    &wasm.LimitsType{},
			expected: []byte{0x0, 0},
		},
		{
			name:     "min 0, max 0",
			input:    &wasm.LimitsType{Max: &zero},
			expected: []byte{0x1, 0, 0},
		},
		{
			name:     "min largest",
			input:    &wasm.LimitsType{Min: max},
			expected: []byte{0x0, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
		{
			name:     "min 0, max largest",
			input:    &wasm.LimitsType{Max: &max},
			expected: []byte{0x1, 0, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
		{
			name:     "min largest max largest",
			input:    &wasm.LimitsType{Min: max, Max: &max},
			expected: []byte{0x1, 0xff, 0xff, 0xff, 0xff, 0xf, 0xff, 0xff, 0xff, 0xff, 0xf},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := encodeLimitsType(tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			decoded, err := decodeLimitsType(bytes.NewReader(b))
			require.NoError(t, err)
			require.Equal(t, decoded, tc.input)
		})
	}
}
