package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionType_Encode(t *testing.T) {
	i32, i64 := ValueTypeI32, ValueTypeI64
	tests := []struct {
		name     string
		input    *FunctionType
		expected []byte
	}{
		{
			name:     "empty",
			input:    &FunctionType{},
			expected: []byte{0x60, 0, 0},
		},
		{
			name:     "one param no result",
			input:    &FunctionType{Params: []ValueType{i32}},
			expected: []byte{0x60, 1, i32, 0},
		},
		{
			name:     "undefined param no result", // ensure future spec changes don't panic
			input:    &FunctionType{Params: []ValueType{0x7b}},
			expected: []byte{0x60, 1, 0x7b, 0},
		},
		{
			name:     "no param one result",
			input:    &FunctionType{Results: []ValueType{i32}},
			expected: []byte{0x60, 0, 1, i32},
		},
		{
			name:     "no param undefined result", // ensure future spec changes don't panic
			input:    &FunctionType{Results: []ValueType{0x7b}},
			expected: []byte{0x60, 0, 1, 0x7b},
		},
		{
			name:     "one param one result",
			input:    &FunctionType{Params: []ValueType{i64}, Results: []ValueType{i32}},
			expected: []byte{0x60, 1, i64, 1, i32},
		},
		{
			name:     "undefined param undefined result", // ensure future spec changes don't panic
			input:    &FunctionType{Params: []ValueType{0x7b}, Results: []ValueType{0x7b}},
			expected: []byte{0x60, 1, 0x7b, 1, 0x7b},
		},
		{
			name:     "two params no result",
			input:    &FunctionType{Params: []ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 0},
		},
		{
			name:     "no param two results", // this is just for coverage as WebAssembly 1.0 (MVP) does not allow it!
			input:    &FunctionType{Results: []ValueType{i32, i64}},
			expected: []byte{0x60, 0, 2, i32, i64},
		},
		{
			name:     "one param two results", // this is just for coverage as WebAssembly 1.0 (MVP) does not allow it!
			input:    &FunctionType{Params: []ValueType{i64}, Results: []ValueType{i32, i64}},
			expected: []byte{0x60, 1, i64, 2, i32, i64},
		},
		{
			name:     "two param one result",
			input:    &FunctionType{Params: []ValueType{i32, i64}, Results: []ValueType{i32}},
			expected: []byte{0x60, 2, i32, i64, 1, i32},
		},
		{
			name:     "two param two results", // this is just for coverage as WebAssembly 1.0 (MVP) does not allow it!
			input:    &FunctionType{Params: []ValueType{i32, i64}, Results: []ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 2, i32, i64},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := tc.input.encode()
			require.Equal(t, tc.expected, bytes)
		})
	}
}

func TestFunctionType_String(t *testing.T) {
	tp := FunctionType{}
	require.Equal(t, "null_null", tp.String())

	// With params.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}}
	require.Equal(t, "i32_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i32f64_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "f32i32f64_null", tp.String())

	// With results.
	tp = FunctionType{Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "null_i64", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "null_i64f32", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "null_f32i32f64", tp.String())

	// With params and results.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "i32_i64", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32}, Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "i64f32_i64f32", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32, ValueTypeF64}, Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i64f32f64_f32i32f64", tp.String())
}
