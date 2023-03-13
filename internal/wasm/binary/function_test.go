package binary

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestFunctionType(t *testing.T) {
	i32, i64, funcRef, externRef := wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeFuncref, wasm.ValueTypeExternref
	tests := []struct {
		name     string
		input    wasm.FunctionType
		expected []byte
	}{
		{
			name:     "empty",
			input:    wasm.FunctionType{},
			expected: []byte{0x60, 0, 0},
		},
		{
			name:     "one param no result",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32}},
			expected: []byte{0x60, 1, i32, 0},
		},
		{
			name:     "no param one result",
			input:    wasm.FunctionType{Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 0, 1, i32},
		},
		{
			name:     "one param one result",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 1, i64, 1, i32},
		},
		{
			name:     "two params no result",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 0},
		},
		{
			name:     "two param one result",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32}},
			expected: []byte{0x60, 2, i32, i64, 1, i32},
		},
		{
			name:     "no param two results",
			input:    wasm.FunctionType{Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 0, 2, i32, i64},
		},
		{
			name:     "one param two results",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 1, i64, 2, i32, i64},
		},
		{
			name:     "two param two results",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32, i64}},
			expected: []byte{0x60, 2, i32, i64, 2, i32, i64},
		},
		{
			name:     "two param two results with funcrefs",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32, funcRef}, Results: []wasm.ValueType{funcRef, i64}},
			expected: []byte{0x60, 2, i32, funcRef, 2, funcRef, i64},
		},
		{
			name:     "two param two results with externrefs",
			input:    wasm.FunctionType{Params: []wasm.ValueType{i32, externRef}, Results: []wasm.ValueType{externRef, i64}},
			expected: []byte{0x60, 2, i32, externRef, 2, externRef, i64},
		},
	}

	for _, tt := range tests {
		tc := tt

		b := binaryencoding.EncodeFunctionType(&tc.input)
		t.Run(fmt.Sprintf("encode - %s", tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, b)
		})

		t.Run(fmt.Sprintf("decode - %s", tc.name), func(t *testing.T) {
			var actual wasm.FunctionType
			err := decodeFunctionType(api.CoreFeaturesV2, bytes.NewReader(b), &actual)
			require.NoError(t, err)
			// Set the FunctionType key on the input.
			_ = tc.input.String()
			require.Equal(t, actual, tc.input)
		})
	}
}

func TestDecodeFunctionType_Errors(t *testing.T) {
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64
	tests := []struct {
		name            string
		input           []byte
		enabledFeatures api.CoreFeatures
		expectedErr     string
	}{
		{
			name:        "undefined param no result",
			input:       []byte{0x60, 1, 0x6e, 0},
			expectedErr: "could not read parameter types: invalid value type: 110",
		},
		{
			name:        "no param undefined result",
			input:       []byte{0x60, 0, 1, 0x6e},
			expectedErr: "could not read result types: invalid value type: 110",
		},
		{
			name:        "undefined param undefined result",
			input:       []byte{0x60, 1, 0x6e, 1, 0x6e},
			expectedErr: "could not read parameter types: invalid value type: 110",
		},
		{
			name:        "no param two results - multi-value not enabled",
			input:       []byte{0x60, 0, 2, i32, i64},
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:        "one param two results - multi-value not enabled",
			input:       []byte{0x60, 1, i64, 2, i32, i64},
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:        "two param two results - multi-value not enabled",
			input:       []byte{0x60, 2, i32, i64, 2, i32, i64},
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var actual wasm.FunctionType
			err := decodeFunctionType(api.CoreFeaturesV1, bytes.NewReader(tc.input), &actual)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
