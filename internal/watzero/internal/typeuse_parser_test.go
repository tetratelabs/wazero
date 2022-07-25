package internal

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type typeUseParserTest struct {
	name                string
	input               string
	expectedInlinedType *wasm.FunctionType
	expectedTypeIdx     wasm.Index
	expectedParamNames  wasm.NameMap

	expectedOnTypeUsePosition callbackPosition
	expectedOnTypeUseToken    tokenType
	expectedTrailingTokens    []tokenType
}

func TestTypeUseParser_InlinesTypesWhenNotYetAdded(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:                "empty",
			input:               "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param no result",
			input:               "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param no result - ID",
			input:               "((param $x i32))",
			expectedInlinedType: i32_v,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}},
		},
		{
			name:                "result no param",
			input:               "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "mixed param no result",
			input:               "((param i32) (param i64))",
			expectedInlinedType: i32i64_v,
		},
		{
			name:                "mixed param no result - ID",
			input:               "((param $x i32) (param $y i64))",
			expectedInlinedType: i32i64_v,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "mixed param result",
			input:               "((param i32) (param i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "mixed param result - ID",
			input:               "((param $x i32) (param $y i64) (result i32))",
			expectedInlinedType: i32i64_i32,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "abbreviated param result",
			input:               "((param i32 i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "mixed param abbreviation", // Verifies we can handle less param fields than param types
			input:               "((param i32 i32) (param i32) (param i64) (param f32))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
		},

		// Below are changes to test/core/br.wast from the commit that added "multi-value" support.
		// See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c

		{
			name:                "multi-value - v_i64f32 abbreviated",
			input:               "((result i64 f32))",
			expectedInlinedType: &wasm.FunctionType{Results: []wasm.ValueType{i64, f32}},
		},
		{
			name:                "multi-value - i32i64_f32f64 abbreviated",
			input:               "((param i32 i64) (result f32 f64))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
		},
		{
			name:                "multi-value - v_i64f32",
			input:               "((result i64) (result f32))",
			expectedInlinedType: &wasm.FunctionType{Results: []wasm.ValueType{i64, f32}},
		},
		{
			name:                "multi-value - i32i64_f32f64",
			input:               "((param i32) (param i64) (result f32) (result f64))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
		},
		{
			name:                "multi-value - i32i64_f32f64 named",
			input:               "((param $x i32) (param $y i64) (result f32) (result f64))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "multi-value - i64i64f32_f32i32 results abbreviated in groups",
			input:               "((result i64 i64 f32) (result f32 i32))",
			expectedInlinedType: &wasm.FunctionType{Results: []wasm.ValueType{i64, i64, f32, f32, i32}},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 params and results abbreviated in groups",
			input: "((param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32))",
			expectedInlinedType: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 abbreviated in groups",
			input: "((param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32))",
			expectedInlinedType: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 abbreviated in groups",
			input: "((param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32))",
			expectedInlinedType: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - empty abbreviated results",
			input: "((result) (result) (result i64 i64) (result) (result f32) (result))",
			// Abbreviations have min length zero, which implies no-op results are ok.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A2
			expectedInlinedType: &wasm.FunctionType{Results: []wasm.ValueType{i64, i64, f32}},
		},
		{
			name: "multi-value - empty abbreviated params and results",
			input: `(
  (param i32 i32) (param i64 i32) (param) (param $x i32) (param)
  (result) (result f32 f64) (result f64 i32) (result)
)`,
			// Abbreviations have min length zero, which implies no-op results are ok.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A2
			expectedInlinedType: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
			expectedParamNames: wasm.NameMap{&wasm.NameAssoc{Index: 4, Name: "x"}},
		},
	}

	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		module := &wasm.Module{}
		tp := newTypeUseParser(wasm.Features20220419, module, newIndexNamespace(module.SectionElementCount))
		return tp, func(t *testing.T) {
			// We should have inlined the type, and it is the first type use, which means the inlined index is zero
			require.Zero(t, tp.inlinedTypeIndices[0].inlinedIdx)
			require.Equal(t, []*wasm.FunctionType{tc.expectedInlinedType}, tp.inlinedTypes)
		}
	})
}

func TestTypeUseParser_UnresolvedType(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:            "unresolved type - index",
			input:           "((type 1))",
			expectedTypeIdx: 1,
		},
		{
			name:            "unresolved type - ID",
			input:           "((type $v_v))",
			expectedTypeIdx: 0,
		},
		{
			name:                "unresolved type - index - match",
			input:               "((type 3) (param i32 i64) (result i32))",
			expectedTypeIdx:     3,
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "unresolved type - ID - match",
			input:               "((type $i32i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx:     0,
			expectedInlinedType: i32i64_i32,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		module := &wasm.Module{}
		tp := newTypeUseParser(wasm.Features20220419, module, newIndexNamespace(module.SectionElementCount))
		return tp, func(t *testing.T) {
			require.NotNil(t, tp.typeNamespace.unresolvedIndices)
			if tc.expectedInlinedType == nil {
				require.Zero(t, len(tp.inlinedTypes), "expected no inlinedTypes")
			} else {
				require.Equal(t, tc.expectedInlinedType, tp.inlinedTypes[0])
			}
		}
	})
}

func TestTypeUseParser_ReuseExistingType(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:            "match existing - result",
			input:           "((result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "match existing - nullary",
			input:           "()",
			expectedTypeIdx: 1,
		},
		{
			name:            "match existing - param",
			input:           "((param i32))",
			expectedTypeIdx: 2,
		},
		{
			name:            "match existing - param and result",
			input:           "((param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field index - result",
			input:           "((type 0))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field ID - result",
			input:           "((type $v_i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field ID - result - match",
			input:           "((type $v_i32) (result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field index - nullary",
			input:           "((type 1))",
			expectedTypeIdx: 1,
		},
		{
			name:            "type field ID - nullary",
			input:           "((type $v_v))",
			expectedTypeIdx: 1,
		},
		{
			name:            "type field index - param",
			input:           "((type 2))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field ID - param",
			input:           "((type $i32_v))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field ID - param - match",
			input:           "((type $i32_v) (param i32))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field index - param and result",
			input:           "((type 3))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field ID - param and result",
			input:           "((type $i32i64_i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field ID - param and result - matched",
			input:           "((type $i32i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		// Add types to cover the main ways types uses are declared
		module := &wasm.Module{TypeSection: []*wasm.FunctionType{v_i32, v_v, i32_v, i32i64_i32}}
		typeNamespace := newIndexNamespace(module.SectionElementCount)
		_, err := typeNamespace.setID([]byte("$v_i32"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$v_v"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$i32_v"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$i32i64_i32"))
		require.NoError(t, err)
		typeNamespace.count++

		tp := newTypeUseParser(wasm.Features20220419, module, typeNamespace)
		return tp, func(t *testing.T) {
			require.Zero(t, len(tp.typeNamespace.unresolvedIndices))
			require.Zero(t, len(tp.inlinedTypes))
			require.Zero(t, len(tp.inlinedTypeIndices))
		}
	})
}

func TestTypeUseParser_ReuseExistingInlinedType(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:                "match existing - result",
			input:               "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "nullary",
			input:               "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param",
			input:               "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param and result",
			input:               "((param i32 i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		module := &wasm.Module{}
		tp := newTypeUseParser(wasm.Features20220419, module, newIndexNamespace(module.SectionElementCount))
		// inline a type that doesn't match the test
		require.NoError(t, parseTypeUse(tp, "((param i32 i64))", ignoreTypeUse))
		// inline the test type
		require.NoError(t, parseTypeUse(tp, tc.input, ignoreTypeUse))

		return tp, func(t *testing.T) {
			// verify it wasn't duplicated
			require.Equal(t, []*wasm.FunctionType{i32i64_v, tc.expectedInlinedType}, tp.inlinedTypes)
			// last two inlined types are the same
			require.Equal(t, tp.inlinedTypeIndices[1].inlinedIdx, tp.inlinedTypeIndices[2].inlinedIdx)
		}
	})
}

func TestTypeUseParser_BeginResets(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:                "result",
			input:               "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "nullary",
			input:               "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param",
			input:               "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param and result",
			input:               "((param i32 i32) (result i32))",
			expectedInlinedType: i32i32_i32,
		},
		{
			name:                "param and result - with IDs",
			input:               "((param $l i32) (param $r i32) (result i32))",
			expectedInlinedType: i32i32_i32,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "l"}, &wasm.NameAssoc{Index: 1, Name: "r"}},
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		module := &wasm.Module{}
		tp := newTypeUseParser(wasm.Features20220419, module, newIndexNamespace(module.SectionElementCount))
		// inline a type that uses all fields
		require.NoError(t, parseTypeUse(tp, "((type $i32i64_i32) (param $x i32) (param $y i64) (result i32))", ignoreTypeUse))
		require.NoError(t, parseTypeUse(tp, tc.input, ignoreTypeUse))

		return tp, func(t *testing.T) {
			// this is the second inlined type
			require.Equal(t, []*wasm.FunctionType{i32i64_i32, tc.expectedInlinedType}, tp.inlinedTypes)
		}
	})
}

type typeUseTestFunc func(*typeUseParserTest) (*typeUseParser, func(t *testing.T))

// To prevent having to maintain a lot of tests to cover the necessary dimensions, this generates combinations that
// can happen in a type use.
// Ex. (func ) (func (local i32)) and (func nop) are all empty type uses, and we need to hit all three cases
func runTypeUseParserTests(t *testing.T, tests []*typeUseParserTest, tf typeUseTestFunc) {
	moreTests := make([]*typeUseParserTest, 0, len(tests)*2)
	for _, tt := range tests {
		tt.expectedOnTypeUsePosition = callbackPositionEndField
		tt.expectedOnTypeUseToken = tokenRParen // at the end of the field ')'
		tt.expectedTrailingTokens = nil

		kt := *tt // copy
		kt.name = fmt.Sprintf("%s - trailing keyword", tt.name)
		kt.input = fmt.Sprintf("%s nop)", tt.input[:len(tt.input)-1])
		kt.expectedOnTypeUsePosition = callbackPositionUnhandledToken
		kt.expectedOnTypeUseToken = tokenKeyword // at 'nop' and ')' remains
		kt.expectedTrailingTokens = []tokenType{tokenRParen}
		moreTests = append(moreTests, &kt)

		ft := *tt // copy
		ft.name = fmt.Sprintf("%s - trailing field", tt.name)
		ft.input = fmt.Sprintf("%s (nop))", tt.input[:len(tt.input)-1])
		ft.expectedOnTypeUsePosition = callbackPositionUnhandledField
		ft.expectedOnTypeUseToken = tokenKeyword // at 'nop' and '))' remain
		ft.expectedTrailingTokens = []tokenType{tokenRParen, tokenRParen}
		moreTests = append(moreTests, &ft)
	}

	for _, tt := range append(tests, moreTests...) {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			var parsedTypeIdx wasm.Index
			var parsedParamNames wasm.NameMap
			p := &collectTokenTypeParser{}
			var setTypeUse onTypeUse = func(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
				parsedTypeIdx = typeIdx
				parsedParamNames = paramNames
				require.Equal(t, tc.expectedOnTypeUsePosition, pos)
				require.Equal(t, tc.expectedOnTypeUseToken, tok)
				return p.parse, nil
			}

			tp, test := tf(tc)
			require.NoError(t, parseTypeUse(tp, tc.input, setTypeUse))
			require.Equal(t, tc.expectedTrailingTokens, p.tokenTypes)
			require.Equal(t, tc.expectedTypeIdx, parsedTypeIdx)
			require.Equal(t, tc.expectedParamNames, parsedParamNames)
			test(t)
		})
	}
}

func TestTypeUseParser_Errors(t *testing.T) {
	tests := []struct {
		name, input, expectedErr string
		enabledFeatures          wasm.Features
	}{
		{
			name:        "not param",
			input:       "((param i32) ($param i32))",
			expectedErr: "1:15: unexpected ID: $param",
		},
		{
			name:        "param wrong type",
			input:       "((param i33))",
			expectedErr: "1:9: unknown type: i33",
		},
		{
			name:        "param ID in abbreviation",
			input:       "((param $x i32 i64) ",
			expectedErr: "1:16: cannot assign IDs to parameters in abbreviated form",
		},
		{
			name:        "param second ID",
			input:       "((param $x $x i64) ",
			expectedErr: "1:12: redundant ID $x",
		},
		{
			name:        "param duplicate ID",
			input:       "((param $x i32) (param $x i64) ",
			expectedErr: "1:24: duplicate ID $x",
		},
		{
			name:        "param wrong end",
			input:       `((param i64 ""))`,
			expectedErr: "1:13: unexpected string: \"\"",
		},
		{
			name:        "result has no ID",
			input:       "((result $x i64) ",
			expectedErr: "1:10: unexpected ID: $x",
		},
		{
			name:        "result wrong type",
			input:       "((result i33))",
			expectedErr: "1:10: unknown type: i33",
		},
		{
			name:        "result abbreviated",
			input:       "((result i32 i64))",
			expectedErr: "1:14: multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:        "result twice",
			input:       "((result i32) (result i32))",
			expectedErr: "1:16: multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:            "result second wrong",
			input:           "((result i32) (result i33))",
			enabledFeatures: wasm.Features20220419,
			expectedErr:     "1:23: unknown type: i33",
		},
		{
			name:            "result second redundant type wrong",
			input:           "((result i32) (result i32 i33))",
			enabledFeatures: wasm.Features20220419,
			expectedErr:     "1:27: unknown type: i33",
		},
		{
			name:        "param after result",
			input:       "((result i32) (param i32))",
			expectedErr: "1:16: param after result",
		},
		{
			name:        "type after result",
			input:       "((result i32) (type i32))",
			expectedErr: "1:16: type after result",
		},
		{
			name:        "result wrong end",
			input:       "((result i64 \"\"))",
			expectedErr: "1:14: unexpected string: \"\"",
		},
		{
			name:        "type missing index",
			input:       "((type))",
			expectedErr: "1:7: missing index",
		},
		{
			name:        "type wrong token",
			input:       "((type v_v))",
			expectedErr: "1:8: unexpected keyword: v_v",
		},
		{
			name:        "type redundant",
			input:       "((type 0) (type 1))",
			expectedErr: "1:12: redundant type",
		},
		{
			name:        "type second index",
			input:       "((type 0 1))",
			expectedErr: "1:10: redundant index",
		},
		{
			name:        "type overflow index",
			input:       "((type 4294967296))",
			expectedErr: "1:8: index outside range of uint32: 4294967296",
		},
		{
			name:        "type second ID",
			input:       "((type $v_v $v_v i64) ",
			expectedErr: "1:13: redundant index",
		},

		{
			name:        "type wrong end",
			input:       `((type 0 ""))`,
			expectedErr: "1:10: unexpected string: \"\"",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = wasm.Features20191205
			}
			module := &wasm.Module{}
			tp := newTypeUseParser(enabledFeatures, module, newIndexNamespace(module.SectionElementCount))
			err := parseTypeUse(tp, tc.input, failOnTypeUse)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestTypeUseParser_FailsMatch(t *testing.T) {
	// Add types to cover the main ways types uses are declared
	module := &wasm.Module{TypeSection: []*wasm.FunctionType{v_v, i32i64_i32}}
	typeNamespace := newIndexNamespace(module.SectionElementCount)
	_, err := typeNamespace.setID([]byte("$v_v"))
	require.NoError(t, err)
	typeNamespace.count++

	_, err = typeNamespace.setID([]byte("$i32i64_i32"))
	require.NoError(t, err)
	typeNamespace.count++

	tp := newTypeUseParser(wasm.Features20220419, module, typeNamespace)
	tests := []struct{ name, source, expectedErr string }{
		{
			name:        "nullary index",
			source:      "((type 0)(param i32))",
			expectedErr: "1:21: inlined type doesn't match module.type[0].func",
		},
		{
			name:        "nullary ID",
			source:      "((type $v_v)(param i32))",
			expectedErr: "1:24: inlined type doesn't match module.type[0].func",
		},
		{
			name:        "arity index - fails match",
			source:      "((type 1)(param i32))",
			expectedErr: "1:21: inlined type doesn't match module.type[1].func",
		},
		{
			name:        "arity ID  - fails match",
			source:      "((type $i32i64_i32)(param i32))",
			expectedErr: "1:31: inlined type doesn't match module.type[1].func",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, parseTypeUse(tp, tc.source, failOnTypeUse), tc.expectedErr)
		})
	}
}

var ignoreTypeUse onTypeUse = func(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	return parseNoop, nil
}

var failOnTypeUse onTypeUse = func(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	return nil, errors.New("unexpected to call onTypeUse on error")
}

func parseTypeUse(tp *typeUseParser, source string, onTypeUse onTypeUse) error {
	var parser tokenParser = func(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
		return tp.begin(wasm.SectionIDFunction, onTypeUse, tok, tokenBytes, line, col)
	}

	line, col, err := lex(skipTokens(1, parser), []byte(source))
	if err != nil {
		err = &FormatError{Line: line, Col: col, cause: err}
	}
	return err
}

func TestTypeUseParser_ErrorContext(t *testing.T) {
	p := typeUseParser{currentField: 3}
	tests := []struct {
		source   string
		pos      parserPosition
		expected string
	}{
		{source: "initial", pos: positionInitial, expected: ""},
		{source: "param", pos: positionParam, expected: ".param[3]"},
		{source: "result", pos: positionResult, expected: ".result[3]"},
		{source: "type", pos: positionType, expected: ".type"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.source, func(t *testing.T) {
			p.pos = tc.pos
			require.Equal(t, tc.expected, p.errorContext())
		})
	}
}
