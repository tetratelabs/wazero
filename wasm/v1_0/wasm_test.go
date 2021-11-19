package v1_0

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueType_String(t *testing.T) {
	tests := []struct {
		input    ValueType
		expected string
	}{
		{I32, "i32"},
		{I64, "i64"},
		{F32, "f32"},
		{F64, "f64"},
	}

	for _, tc := range tests {
		tc := tc // pin! see https://github.com/kyoh86/scopelint for why

		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.String())
		})
	}

	t.Run("unexpected", func(t *testing.T) {
		require.Equal(t, "valueType(255)", ValueType(255).String())
	})
}
