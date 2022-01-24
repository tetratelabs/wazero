package text

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestBeginBody(t *testing.T) {
	localGet0End := []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}

	tests := []struct {
		name, input  string
		expectedCode []byte
	}{
		{
			name: "empty",
		},
		{
			name:         "local.get",
			input:        "local.get 0",
			expectedCode: localGet0End,
		},
		{
			name:         "local.get twice",
			input:        "local.get 0 local.get 1",
			expectedCode: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd},
		},
		{
			name:  "local.get twice and add",
			input: "local.get 0 local.get 1 i32.add",
			expectedCode: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeLocalGet, 0x01,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			lp := &funcParser{m: &moduleParser{}, onBodyEnd: noopTokenParser}
			_, _, err := lex(lp.beginBody(), []byte(tc.input))
			require.NoError(t, err)
			if tc.expectedCode == nil {
				require.Equal(t, end, lp.getBody())
			} else {
				require.Equal(t, tc.expectedCode, lp.getBody())
			}
		})
	}
}

func TestBeginBodyField_Errors(t *testing.T) {
	tests := []struct {
		name, input, expectedErr string
	}{
		{
			name:        "fields not yet supported",
			input:       "(f32.const 1.1)",
			expectedErr: "TODO: s-expressions are not yet supported: f32.const",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			lp := &funcParser{m: &moduleParser{}, onBodyEnd: noopTokenParser}
			_, _, err := lex(lp.beginBodyField(), []byte(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestBeginBody_Errors(t *testing.T) {
	tests := []struct {
		name, input, expectedErr string
	}{
		{
			name:        "local.get wrong value",
			input:       "local.get a",
			expectedErr: "unexpected keyword: a",
		},
		{
			name:        "local.get symbolic ID",
			input:       "local.get $y",
			expectedErr: "TODO: index variables are not yet supported",
		},
		{
			name:        "local.get overflow",
			input:       "local.get 4294967296",
			expectedErr: "malformed i32 4294967296: value out of range",
		},
		{
			name:        "instruction not yet supported",
			input:       "f32.const 1.1",
			expectedErr: "unsupported instruction: f32.const",
		},
		{
			name:        "fields not yet supported",
			input:       "(f32.const 1.1)",
			expectedErr: "TODO: s-expressions are not yet supported: f32.const",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			lp := &funcParser{m: &moduleParser{}, onBodyEnd: noopTokenParser}
			_, _, err := lex(lp.beginBody(), []byte(tc.input))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
