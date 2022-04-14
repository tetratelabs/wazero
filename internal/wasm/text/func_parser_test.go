package text

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
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
			name:     "local.get, drop",
			source:   "(func local.get 0 drop)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeDrop, wasm.OpcodeEnd}},
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
			name:     "i32.const",
			source:   "(func i32.const 306)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeI32Const, 0xb2, 0x02, wasm.OpcodeEnd}},
		},
		{
			name:     "i64.const",
			source:   "(func i64.const 356)",
			expected: &wasm.Code{Body: []byte{wasm.OpcodeI64Const, 0xe4, 0x02, wasm.OpcodeEnd}},
		},
		{
			name:   "i64.load",
			source: "(func i32.const 8 i64.load)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeI32Const, 8, // dynamic memory offset to load
				wasm.OpcodeI64Load, 0x3, 0x0, // load alignment=3 (natural alignment) staticOffset=0
				wasm.OpcodeEnd,
			}},
		},
		{
			name:   "i64.store",
			source: "(func i32.const 8 i64.const 37 i64.store)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeI32Const, 8, // dynamic memory offset to store
				wasm.OpcodeI64Const, 37, // value to store
				wasm.OpcodeI64Store, 0x3, 0x0, // load alignment=3 (natural alignment) staticOffset=0
				wasm.OpcodeEnd,
			}},
		},

		// Below are changes to test/core/i32 and i64.wast from the commit that added "sign-extension-ops" support.
		// See https://github.com/WebAssembly/spec/commit/e308ca2ae04d5083414782e842a81f931138cf2e

		{
			name:   "i32.extend8_s",
			source: "(func (param i32) local.get 0 i32.extend8_s)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI32Extend8S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:   "i32.extend16_s",
			source: "(func (param i32) local.get 0 i32.extend16_s)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI32Extend16S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:   "i64.extend8_s",
			source: "(func (param i64) local.get 0 i64.extend8_s)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI64Extend8S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:   "i64.extend16_s",
			source: "(func (param i64) local.get 0 i64.extend16_s)",
			expected: &wasm.Code{Body: []byte{
				wasm.OpcodeLocalGet, 0x00,
				wasm.OpcodeI64Extend16S,
				wasm.OpcodeEnd,
			}},
		},
		{
			name:   "i64.extend32_s",
			source: "(func (param i64) local.get 0 i64.extend32_s)",
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
			fp := newFuncParser(wasm.FeaturesFinished, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expected, parsedCode)
		})
	}
}

func TestFuncParser_Call_Unresolved(t *testing.T) {
	tests := []struct {
		name, source            string
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
			fp := newFuncParser(wasm.Features20191205, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expectedCode, parsedCode)
			require.Equal(t, []*unresolvedIndex{tc.expectedUnresolvedIndex}, fp.funcNamespace.unresolvedIndices)
		})
	}
}

func TestFuncParser_Call_Resolved(t *testing.T) {
	tests := []struct {
		name, source string
		expected     *wasm.Code
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

			fp := newFuncParser(wasm.FeaturesFinished, &typeUseParser{module: &wasm.Module{}}, funcNamespace, setFunc)
			require.NoError(t, parseFunc(fp, tc.source))
			require.Equal(t, tc.expected, parsedCode)
		})
	}
}

func TestFuncParser_Errors(t *testing.T) {
	tests := []struct {
		name, source string
		expectedErr  string
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
			name:        "i32.const overflow",
			source:      "(func i32.const 4294967296)",
			expectedErr: "1:17: i32 outside range of uint32: 4294967296",
		},
		{
			name:        "i64.const overflow",
			source:      "(func i64.const 18446744073709551616)",
			expectedErr: "1:17: i64 outside range of uint64: 18446744073709551616",
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
			expectedErr: "1:21: multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:        "i32.extend8_s disabled",
			source:      "(func (param i32) local.get 0 i32.extend8_s)",
			expectedErr: "1:31: i32.extend8_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			name:        "i32.extend16_s disabled",
			source:      "(func (param i32) local.get 0 i32.extend16_s)",
			expectedErr: "1:31: i32.extend16_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			name:        "i64.extend8_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend8_s)",
			expectedErr: "1:31: i64.extend8_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			name:        "i64.extend16_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend16_s)",
			expectedErr: "1:31: i64.extend16_s invalid as feature \"sign-extension-ops\" is disabled",
		},
		{
			name:        "i64.extend32_s disabled",
			source:      "(func (param i64) local.get 0 i64.extend32_s)",
			expectedErr: "1:31: i64.extend32_s invalid as feature \"sign-extension-ops\" is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{}
			fp := newFuncParser(wasm.Features20191205, &typeUseParser{module: module}, newIndexNamespace(module.SectionElementCount), failOnFunc)
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
