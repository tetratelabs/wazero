package text

//
//import (
//	"testing"
//
//	"github.com/stretchr/testify/require"
//
//	"github.com/tetratelabs/wazero/wasm"
//)
//
//func TestCodeParser(t *testing.T) {
//	tests := []struct {
//		name, input string
//		expected    *wasm.Code
//	}{
//		{
//			name:     "empty",
//			expected: &wasm.Code{Body: []byte{wasm.OpcodeEnd}},
//		},
//		{
//			name:     "local.get",
//			input:    "local.get 0",
//			expected: &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}},
//		},
//		{
//			name:     "local.get twice",
//			input:    "local.get 0 local.get 1",
//			expected: &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd}},
//		},
//		{
//			name:  "local.get twice and add",
//			input: "local.get 0 local.get 1 i32.add",
//			expected: &wasm.Code{Body: []byte{
//				wasm.OpcodeLocalGet, 0x00,
//				wasm.OpcodeLocalGet, 0x01,
//				wasm.OpcodeI32Add,
//				wasm.OpcodeEnd,
//			}},
//		},
//	}
//
//	for _, tt := range tests {
//		tc := tt
//
//		t.Run(tc.name, func(t *testing.T) {
//			p := &collectTokenParser{}
//			cp := newCodeParser(p.parse)
//			_, _, err := lex(cp.begin, []byte(tc.input))
//			require.NoError(t, err)
//			require.Equal(t, tc.expected, cp.getCode())
//		})
//	}
//}
//
//func TestCodeParser_CallsOnFuncOnRParen(t *testing.T) {
//	p := &collectTokenParser{}
//	cp := newCodeParser(p.parse)
//
//	// codeParser starts after the '(', so we need to eat it first!
//	_, _, err := lex(skipTokens(1, cp.begin), []byte("(local.get 0)"))
//	require.NoError(t, err)
//	require.Equal(t, &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}}, cp.getCode())
//	require.Equal(t, []*token{{
//		tokenType: tokenRParen,
//		line:      1,
//		col:       13,
//		token:     ")",
//	}}, p.tokens)
//}
//
//func TestCodeParser_Errors(t *testing.T) {
//	tests := []struct {
//		name, input, expectedErr string
//	}{
//		{
//			name:        "invalid field",
//			input:       "($local.get 1)",
//			expectedErr: "unexpected ID: $local.get",
//		},
//		{
//			name:        "local.get wrong value",
//			input:       "local.get a",
//			expectedErr: "unexpected keyword: a",
//		},
//		{
//			name:        "local.get symbolic ID",
//			input:       "local.get $y",
//			expectedErr: "TODO: index variables are not yet supported",
//		},
//		{
//			name:        "local.get overflow",
//			input:       "local.get 4294967296",
//			expectedErr: "index outside range of uint32: 4294967296",
//		},
//		{
//			name:        "instruction not yet supported",
//			input:       "f32.const 1.1",
//			expectedErr: "unsupported instruction: f32.const",
//		},
//		{
//			name:        "s-expressions not yet supported",
//			input:       "(f32.const 1.1)",
//			expectedErr: "TODO: s-expressions are not yet supported: f32.const",
//		},
//		{
//			name:        "param out of order", // because this parser is after the type use, so it must be wrong
//			input:       "(param i32)",
//			expectedErr: "param declared out of order",
//		},
//		{
//			name:        "result out of order", // because this parser is after the type use, so it must be wrong
//			input:       "(result i32)",
//			expectedErr: "result declared out of order",
//		},
//	}
//
//	for _, tt := range tests {
//		tc := tt
//
//		t.Run(tc.name, func(t *testing.T) {
//			cp := newCodeParser(parseNoop)
//			_, _, err := lex(cp.begin, []byte(tc.input))
//			require.EqualError(t, err, tc.expectedErr)
//		})
//	}
//}
