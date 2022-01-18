package text

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestParseValueType(t *testing.T) {
	tests := []struct {
		input    string
		expected wasm.ValueType
	}{
		{"i32", wasm.ValueTypeI32},
		{"i64", wasm.ValueTypeI64},
		{"f32", wasm.ValueTypeF32},
		{"f64", wasm.ValueTypeF64},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.input, func(t *testing.T) {
			m, err := parseValueType([]byte(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, m)
		})
	}
	t.Run("unknown type", func(t *testing.T) {
		_, err := parseValueType([]byte("f65"))
		require.EqualError(t, err, "unknown type: f65")
	})
}
