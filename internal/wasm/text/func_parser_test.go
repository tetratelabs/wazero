package text

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestFuncParser(t *testing.T) {
	tests := []struct {
		name, source    string
		enabledFeatures wasm.Features
		expected        *wasm.Code
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
		{
			name:            "i32.extend8_s",
			source:          "(func (param i32) local.get 0 i32.extend8_s)",
			enabledFeatures: wasm.FeatureSignExtensionOps,
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI32Extend8S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:            "i32.extend16_s",
			source:          "(func (param i32) local.get 0 i32.extend16_s)",
			enabledFeatures: wasm.FeatureSignExtensionOps,
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI32Extend16S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:            "i64.extend8_s",
			source:          "(func (param i64) local.get 0 i64.extend8_s)",
			enabledFeatures: wasm.FeatureSignExtensionOps,
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI64Extend8S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:            "i64.extend16_s",
			source:          "(func (param i64) local.get 0 i64.extend16_s)",
			enabledFeatures: wasm.FeatureSignExtensionOps,
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI64Extend16S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:            "i64.extend32_s",
			source:          "(func (param i64) local.get 0 i64.extend32_s)",
			enabledFeatures: wasm.FeatureSignExtensionOps,
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI64Extend32S,
				wasm.OpcodeEnd,
			}},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var parsedCode *wasm.Code
			var setFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error) {
				parsedCode = code
				return parseErr, nil
			}

			module := &wasm.Module{}
			fp := newFuncParser(tc.enabledFeatures, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expected, parsedCode)
		})
	}
}

func TestFuncParser_Call_Unresolved(t *testing.T) {
	tests := []struct {
		name, source            string
		enabledFeatures         wasm.Features
		expectedCode            *wasm.Code
		expectedUnresolvedIndex *unresolvedIndex
	}{
		{
			name:         "index zero",
			source:       "(func call 0)",
			expectedCode: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
			expectedUnresolvedIndex: &unresolvedIndex{
				section:    wasm.SectionIDCode,
				bodyOffset: 1, // second byte is the position Code.Body
				targetIdx:  0, // zero is literally the intended index. because targetID isn't set, this will be read
				line:       1, col: 12,
			},
		},
		{
			name:         "index",
			source:       "(func call 2)",
			expectedCode: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x02, wasm.OpcodeEnd}},
			expectedUnresolvedIndex: &unresolvedIndex{
				section:    wasm.SectionIDCode,
				bodyOffset: 1, // second byte is the position Code.Body
				targetIdx:  2,
				line:       1, col: 12,
			},
		},
		{
			name:         "ID",
			source:       "(func call $main)",
			expectedCode: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
			expectedUnresolvedIndex: &unresolvedIndex{
				section:    wasm.SectionIDCode,
				bodyOffset: 1, // second byte is the position Code.Body
				targetID:   "main",
				line:       1, col: 12,
			},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var parsedCode *wasm.Code
			var setFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error) {
				parsedCode = code
				return parseErr, nil
			}

			module := &wasm.Module{}
			fp := newFuncParser(tc.enabledFeatures, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expectedCode, parsedCode)
			require.Equal(t, []*unresolvedIndex{tc.expectedUnresolvedIndex}, fp.funcNamespace.unresolvedIndices)
		})
	}
}

func TestFuncParser_Call_Resolved(t *testing.T) {
	tests := []struct {
		name, source    string
		enabledFeatures wasm.Features
		expected        *wasm.Code
	}{
		{
			name:     "index zero",
			source:   "(func call 0)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
		},
		{
			name:     "index",
			source:   "(func call 2)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x02, wasm.OpcodeEnd}},
		},
		{
			name:     "ID",
			source:   "(func call $main)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeCall, 0x02, wasm.OpcodeEnd}},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			funcNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
				require.Equal(t, wasm.SectionIDFunction, sectionID)
				return 0
			})
			_, err := funcNamespace.setID([]byte("$not_main"))
			require.NoError(t, err)
			funcNamespace.count++

			_, err = funcNamespace.setID([]byte("$also_not_main"))
			require.NoError(t, err)
			funcNamespace.count++

			_, err = funcNamespace.setID([]byte("$main"))
			require.NoError(t, err)
			funcNamespace.count++

			_, err = funcNamespace.setID([]byte("$still_not_main"))
			require.NoError(t, err)
			funcNamespace.count++

			var parsedCode *wasm.Code
			var setFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error) {
				parsedCode = code
				return parseErr, nil
			}

			fp := newFuncParser(tc.enabledFeatures, &typeUseParser{module: &wasm.Module{}}, funcNamespace, setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expected, parsedCode)
		})
	}
}

func TestFuncParser_Errors(t *testing.T) {
	tests := []struct {
		name, source    string
		enabledFeatures wasm.Features
		expectedErr     string
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
			name:        "param after result",
			source:      "(func (result i32) (param i32))",
			expectedErr: "1:21: param after result",
		},
		{
			name:        "duplicate result",
			source:      "(func (result i32) (result i32))",
			expectedErr: "1:21: at most one result allowed",
		},
		{
			name:        "i32.extend8_s disabled",
			source:      "(func (param i32) local.get 0 i32.extend8_s)",
			expectedErr: "1:31: i32.extend8_s invalid as feature sign-extension-ops is disabled",
		},
		{
			name:        "i32.extend16_s disabled",
			source:      "(func (param i32) local.get 0 i32.extend16_s)",
			expectedErr: "1:31: i32.extend16_s invalid as feature sign-extension-ops is disabled",
		},
		{
			name:        "i64.extend8_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend8_s)",
			expectedErr: "1:31: i64.extend8_s invalid as feature sign-extension-ops is disabled",
		},
		{
			name:        "i64.extend16_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend16_s)",
			expectedErr: "1:31: i64.extend16_s invalid as feature sign-extension-ops is disabled",
		},
		{
			name:        "i64.extend32_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend32_s)",
			expectedErr: "1:31: i64.extend32_s invalid as feature sign-extension-ops is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{}
			fp := newFuncParser(tc.enabledFeatures, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), failOnFunc)
			require.EqualError(t, parseFunc(fp, tc.source), tc.expectedErr)
		})
	}
}

var failOnFunc onFunc = func(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error) {
	return nil, errors.New("unexpected to call onFunc on error")
}

func parseFunc(fp *funcParser, source string) error {
	line, col, err := lex(skipTokens(2, fp.begin), []byte(source)) // skip the leading (func
	if err != nil {
		err = &FormatError{Line: line, Col: col, cause: err}
	}
	return err
}
