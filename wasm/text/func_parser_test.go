package text

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestFuncParser(t *testing.T) {
	tests := []struct {
		name, source string
		expected     *wasm.Code
	}{
		{
			name:     "empty",
			source:   "(func)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeEnd}},
		},
		{
			name:     "local.get",
			source:   "(func local.get 0)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeEnd}},
		},
		{
			name:     "local.get twice",
			source:   "(func local.get 0 local.get 1)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd}},
		},
		{
			name:   "local.get twice and add",
			source: "(func local.get 0 local.get 1 i32.add)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeLocalGet, 0x01,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			}},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var parsedCode *wasm.Code
			var setFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, localNames wasm.NameMap) (tokenParser, error) {
				parsedCode = code
				return parseErr, nil
			}

			require.NoError(t, parseFunc(newFuncParser(setFunc), tc.source))
			require.Equal(t, tc.expected, parsedCode)
		})
	}
}

func TestFuncParser_Errors(t *testing.T) {
	tests := []struct {
		name, source, expectedErr string
	}{
		{
			name:        "not field",
			source:      "(func ($local.get 1))",
			expectedErr: "1:8: unexpected ID: $local.get",
		},
		{
			name:        "local.get wrong value",
			source:      "(func local.get a)",
			expectedErr: "1:17: unexpected keyword: a",
		},
		{
			name:        "local.get symbolic ID",
			source:      "(func local.get $y)",
			expectedErr: "1:17: TODO: index variables are not yet supported",
		},
		{
			name:        "local.get overflow",
			source:      "(func local.get 4294967296)",
			expectedErr: "1:17: index outside range of uint32: 4294967296",
		},
		{
			name:        "instruction not yet supported",
			source:      "(func f32.const 1.1)",
			expectedErr: "1:7: unsupported instruction: f32.const",
		},
		{
			name:        "s-expressions not yet supported",
			source:      "(func (f32.const 1.1))",
			expectedErr: "1:8: TODO: s-expressions are not yet supported: f32.const",
		},
		{
			name:        "param out of order", // because this parser is after the type use, so it must be wrong
			source:      "(func (param i32))",
			expectedErr: "1:8: param declared out of order",
		},
		{
			name:        "result out of order", // because this parser is after the type use, so it must be wrong
			source:      "(func (result i32))",
			expectedErr: "1:8: result declared out of order",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			fp := newFuncParser(failOnFunc)
			require.EqualError(t, parseFunc(fp, tc.source), tc.expectedErr)
		})
	}
}

var failOnFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, localNames wasm.NameMap) (tokenParser, error) {
	return nil, errors.New("unexpected to call onFunc on error")
}

func parseFunc(fp *funcParser, source string) error {
	// TODO: all func hooks into func_parser.go, so that we don't have to fake the onTypeUse position
	var parser tokenParser = func(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
		return fp.begin(0, nil, onTypeUseUnhandledToken, tok, tokenBytes, line, col)
	}

	line, col, err := lex(skipTokens(2, parser), []byte(source)) // skip the leading (func
	if err != nil {
		err = &FormatError{Line: line, Col: col, cause: err}
	}
	return err
}
