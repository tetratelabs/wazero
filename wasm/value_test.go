package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeValTypes(t *testing.T) {
	i32, i64, f32, f64 := ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64
	tests := []struct {
		name     string
		input    []ValueType
		expected []byte
	}{
		{
			name:     "empty",
			input:    []ValueType{},
			expected: []byte{0},
		},
		{
			name:     "undefined", // ensure future spec changes don't panic
			input:    []ValueType{0x6f},
			expected: []byte{1, 0x6f},
		},
		{
			name:     "i32",
			input:    []ValueType{i32},
			expected: []byte{1, i32},
		},
		{
			name:     "i64",
			input:    []ValueType{i64},
			expected: []byte{1, i64},
		},
		{
			name:     "f32",
			input:    []ValueType{f32},
			expected: []byte{1, f32},
		},
		{
			name:     "f64",
			input:    []ValueType{f64},
			expected: []byte{1, f64},
		},
		{
			name:     "i32i64",
			input:    []ValueType{i32, i64},
			expected: []byte{2, i32, i64},
		},
		{
			name:     "i32i64f32",
			input:    []ValueType{i32, i64, f32},
			expected: []byte{3, i32, i64, f32},
		},
		{
			name:     "i32i64f32f64",
			input:    []ValueType{i32, i64, f32, f64},
			expected: []byte{4, i32, i64, f32, f64},
		},
		{
			name:     "i32i64f32f64i32",
			input:    []ValueType{i32, i64, f32, f64, i32},
			expected: []byte{5, i32, i64, f32, f64, i32},
		},
		{
			name:     "i32i64f32f64i32i64",
			input:    []ValueType{i32, i64, f32, f64, i32, i64},
			expected: []byte{6, i32, i64, f32, f64, i32, i64},
		},
		{
			name:     "i32i64f32f64i32i64f32",
			input:    []ValueType{i32, i64, f32, f64, i32, i64, f32},
			expected: []byte{7, i32, i64, f32, f64, i32, i64, f32},
		},
		{
			name:     "i32i64f32f64i32i64f32f64",
			input:    []ValueType{i32, i64, f32, f64, i32, i64, f32, f64},
			expected: []byte{8, i32, i64, f32, f64, i32, i64, f32, f64},
		},
		{
			name:     "i32i64f32f64i32i64f32f64i32",
			input:    []ValueType{i32, i64, f32, f64, i32, i64, f32, f64, i32},
			expected: []byte{9, i32, i64, f32, f64, i32, i64, f32, f64, i32},
		},
		{
			name:     "i32i64f32f64i32i64f32f64i32i64",
			input:    []ValueType{i32, i64, f32, f64, i32, i64, f32, f64, i32, i64},
			expected: []byte{10, i32, i64, f32, f64, i32, i64, f32, f64, i32, i64},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeValTypes(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
