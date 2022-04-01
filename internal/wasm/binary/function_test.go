package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeFunctionType(t *testing.T) {
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64
	tests := []struct {
		name     string
		input    *wasm.FunctionType
		expected []byte
	}{
		{
			name:     "empty",
			input:    &wasm.FunctionType{},
			expected: []byte{0x60, 0, 0},
		},
		{
			name:     "one param no result",
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i32}},
			expected: []byte{0x60, 1, i32, 0},
		},
		{
			name:     "undefined param no result", // ensure future spec changes don't panic
			input:    &wasm.FunctionType{Params: []wasm.ValueType{0x6f}},
			expected: []byte{0x60, 1, 0x6f, 0},
		},
		{
			name:     "no param one result",
			input:    &wasm.FunctionType{Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 0, 1, i32},
		},
		{
			name:     "no param undefined result", // ensure future spec changes don't panic
			input:    &wasm.FunctionType{Results: []wasm.ValueType{0x6f}},
			expected: []byte{0x60, 0, 1, 0x6f},
		},
		{
			name:     "one param one result",
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 1, i64, 1, i32},
		},
		{
			name:     "undefined param undefined result", // ensure future spec changes don't panic
			input:    &wasm.FunctionType{Params: []wasm.ValueType{0x6f}, Results: []wasm.ValueType{0x6f}},
			expected: []byte{0x60, 1, 0x6f, 1, 0x6f},
		},
		{
			name:     "two params no result",
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 0},
		},
		{
			name:     "no param two results", // this is just for coverage as WebAssembly 1.0 (20191205) does not allow it!
			input:    &wasm.FunctionType{Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 0, 2, i32, i64},
		},
		{
			name:     "one param two results", // this is just for coverage as WebAssembly 1.0 (20191205) does not allow it!
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 1, i64, 2, i32, i64},
		},
		{
			name:     "two param one result",
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 2, i32, i64, 1, i32},
		},
		{
			name:     "two param two results", // this is just for coverage as WebAssembly 1.0 (20191205) does not allow it!
			input:    &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 2, i32, i64},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeFunctionType(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
