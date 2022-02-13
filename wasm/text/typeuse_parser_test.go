package text

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

type typeUseParserTest struct {
	name                string
	source              string
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
			source:              "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param no result",
			source:              "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param no result - ID",
			source:              "((param $x i32))",
			expectedInlinedType: i32_v,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}},
		},
		{
			name:                "result no param",
			source:              "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "mixed param no result",
			source:              "((param i32) (param i64))",
			expectedInlinedType: i32i64_v,
		},
		{
			name:                "mixed param no result - ID",
			source:              "((param $x i32) (param $y i64))",
			expectedInlinedType: i32i64_v,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "mixed param result",
			source:              "((param i32) (param i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "mixed param result - ID",
			source:              "((param $x i32) (param $y i64) (result i32))",
			expectedInlinedType: i32i64_i32,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "abbreviated param result",
			source:              "((param i32 i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "mixed param abbreviation", // Verifies we can handle less param fields than param types
			source:              "((param i32 i32) (param i32) (param i64) (param f32))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
		},
	}

	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
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
			source:          "((type 1))",
			expectedTypeIdx: 1,
		},
		{
			name:            "unresolved type - ID",
			source:          "((type $v_v))",
			expectedTypeIdx: 0,
		},
		{
			name:                "unresolved type - index - match",
			source:              "((type 3) (param i32 i64) (result i32))",
			expectedTypeIdx:     3,
			expectedInlinedType: i32i64_i32,
		},
		{
			name:                "unresolved type - ID - match",
			source:              "((type $i32i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx:     0,
			expectedInlinedType: i32i64_i32,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
		return tp, func(t *testing.T) {
			require.NotNil(t, tp.typeNamespace.unresolvedIndices)
			if tc.expectedInlinedType == nil {
				require.Empty(t, tp.inlinedTypes)
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
			source:          "((result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "match existing - nullary",
			source:          "()",
			expectedTypeIdx: 1,
		},
		{
			name:            "match existing - param",
			source:          "((param i32))",
			expectedTypeIdx: 2,
		},
		{
			name:            "match existing - param and result",
			source:          "((param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field index - result",
			source:          "((type 0))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field ID - result",
			source:          "((type $v_i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field ID - result - match",
			source:          "((type $v_i32) (result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "type field index - nullary",
			source:          "((type 1))",
			expectedTypeIdx: 1,
		},
		{
			name:            "type field ID - nullary",
			source:          "((type $v_v))",
			expectedTypeIdx: 1,
		},
		{
			name:            "type field index - param",
			source:          "((type 2))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field ID - param",
			source:          "((type $i32_v))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field ID - param - match",
			source:          "((type $i32_v) (param i32))",
			expectedTypeIdx: 2,
		},
		{
			name:            "type field index - param and result",
			source:          "((type 3))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field ID - param and result",
			source:          "((type $i32i64_i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "type field ID - param and result - matched",
			source:          "((type $i32i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		typeNamespace := newIndexNamespace()

		// Add types to cover the main ways types uses are declared
		module := &wasm.Module{TypeSection: []*wasm.FunctionType{v_i32, v_v, i32_v, i32i64_i32}}
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

		tp := newTypeUseParser(module, typeNamespace)
		return tp, func(t *testing.T) {
			require.Nil(t, tp.typeNamespace.unresolvedIndices)
			require.Nil(t, tp.inlinedTypes)
			require.Nil(t, tp.inlinedTypeIndices)
		}
	})
}

func TestTypeUseParser_ReuseExistingInlinedType(t *testing.T) {
	tests := []*typeUseParserTest{
		{
			name:                "match existing - result",
			source:              "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "nullary",
			source:              "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param",
			source:              "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param and result",
			source:              "((param i32 i64) (result i32))",
			expectedInlinedType: i32i64_i32,
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
		// inline a type that doesn't match the test
		require.NoError(t, parseTypeUse(tp, "((param i32 i64))", ignoreTypeUse))
		// inline the test type
		require.NoError(t, parseTypeUse(tp, tc.source, ignoreTypeUse))

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
			source:              "((result i32))",
			expectedInlinedType: v_i32,
		},
		{
			name:                "nullary",
			source:              "()",
			expectedInlinedType: v_v,
		},
		{
			name:                "param",
			source:              "((param i32))",
			expectedInlinedType: i32_v,
		},
		{
			name:                "param and result",
			source:              "((param i32 i32) (result i32))",
			expectedInlinedType: i32i32_i32,
		},
		{
			name:                "param and result - with IDs",
			source:              "((param $l i32) (param $r i32) (result i32))",
			expectedInlinedType: i32i32_i32,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "l"}, &wasm.NameAssoc{Index: 1, Name: "r"}},
		},
	}
	runTypeUseParserTests(t, tests, func(tc *typeUseParserTest) (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
		// inline a type that uses all fields
		require.NoError(t, parseTypeUse(tp, "((type $i32i64_i32) (param $x i32) (param $y i64) (result i32))", ignoreTypeUse))
		require.NoError(t, parseTypeUse(tp, tc.source, ignoreTypeUse))

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
		kt.source = fmt.Sprintf("%s nop)", tt.source[:len(tt.source)-1])
		kt.expectedOnTypeUsePosition = callbackPositionUnhandledToken
		kt.expectedOnTypeUseToken = tokenKeyword // at 'nop' and ')' remains
		kt.expectedTrailingTokens = []tokenType{tokenRParen}
		moreTests = append(moreTests, &kt)

		ft := *tt // copy
		ft.name = fmt.Sprintf("%s - trailing field", tt.name)
		ft.source = fmt.Sprintf("%s (nop))", tt.source[:len(tt.source)-1])
		// Two outcomes, we've reached a field not named "type", "param" or "result" or we completed "result"
		if strings.Contains(tt.source, "result") {
			ft.expectedOnTypeUsePosition = callbackPositionUnhandledToken
			ft.expectedOnTypeUseToken = tokenLParen // at '(' and 'nop))' remain
			ft.expectedTrailingTokens = []tokenType{tokenKeyword, tokenRParen, tokenRParen}
		} else {
			ft.expectedOnTypeUsePosition = callbackPositionUnhandledField
			ft.expectedOnTypeUseToken = tokenKeyword // at 'nop' and '))' remain
			ft.expectedTrailingTokens = []tokenType{tokenRParen, tokenRParen}
		}
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
			require.NoError(t, parseTypeUse(tp, tc.source, setTypeUse))
			require.Equal(t, tc.expectedTrailingTokens, p.tokenTypes)
			require.Equal(t, tc.expectedTypeIdx, parsedTypeIdx)
			require.Equal(t, tc.expectedParamNames, parsedParamNames)
			test(t)
		})
	}
}

func TestTypeUseParser_Errors(t *testing.T) {
	tests := []struct{ name, source, expectedErr string }{
		{
			name:        "not param",
			source:      "((param i32) ($param i32))",
			expectedErr: "1:15: unexpected ID: $param",
		},
		{
			name:        "param missing type",
			source:      "((param))",
			expectedErr: "1:8: expected a type",
		},
		{
			name:        "param wrong type",
			source:      "((param i33))",
			expectedErr: "1:9: unknown type: i33",
		},
		{
			name:        "param ID in abbreviation",
			source:      "((param $x i32 i64) ",
			expectedErr: "1:16: cannot assign IDs to parameters in abbreviated form",
		},
		{
			name:        "param second ID",
			source:      "((param $x $x i64) ",
			expectedErr: "1:12: redundant ID $x",
		},
		{
			name:        "param duplicate ID",
			source:      "((param $x i32) (param $x i64) ",
			expectedErr: "1:24: duplicate ID $x",
		},
		{
			name:        "param wrong end",
			source:      `((param i64 ""))`,
			expectedErr: "1:13: unexpected string: \"\"",
		},
		{
			name:        "result has no ID",
			source:      "((result $x i64) ",
			expectedErr: "1:10: unexpected ID: $x",
		},
		{
			name:        "result missing type",
			source:      "((result))",
			expectedErr: "1:9: expected a type",
		},
		{
			name:        "result wrong type",
			source:      "((result i33))",
			expectedErr: "1:10: unknown type: i33",
		},
		{
			name:        "result second type",
			source:      "((result i32 i64))",
			expectedErr: "1:14: redundant type",
		},
		{
			name:        "result wrong end",
			source:      `((result i64 ""))`,
			expectedErr: "1:14: unexpected string: \"\"",
		},
		{
			name:        "type missing index",
			source:      "((type))",
			expectedErr: "1:7: missing index",
		},
		{
			name:        "type wrong token",
			source:      "((type v_v))",
			expectedErr: "1:8: unexpected keyword: v_v",
		},
		{
			name:        "type redundant",
			source:      "((type 0) (type 1))",
			expectedErr: "1:12: redundant type",
		},
		{
			name:        "type second index",
			source:      "((type 0 1))",
			expectedErr: "1:10: redundant index",
		},
		{
			name:        "type overflow index",
			source:      "((type 4294967296))",
			expectedErr: "1:8: index outside range of uint32: 4294967296",
		},
		{
			name:        "type second ID",
			source:      "((type $v_v $v_v i64) ",
			expectedErr: "1:13: redundant index",
		},

		{
			name:        "type wrong end",
			source:      `((type 0 ""))`,
			expectedErr: "1:10: unexpected string: \"\"",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := parseTypeUse(newTypeUseParser(&wasm.Module{}, newIndexNamespace()), tc.source, failOnTypeUse)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestTypeUseParser_FailsMatch(t *testing.T) {
	typeNamespace := newIndexNamespace()

	// Add types to cover the main ways types uses are declared
	module := &wasm.Module{TypeSection: []*wasm.FunctionType{v_v, i32i64_i32}}
	_, err := typeNamespace.setID([]byte("$v_v"))
	require.NoError(t, err)
	typeNamespace.count++

	_, err = typeNamespace.setID([]byte("$i32i64_i32"))
	require.NoError(t, err)
	typeNamespace.count++

	tp := newTypeUseParser(module, typeNamespace)
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
		return tp.begin(wasm.SectionIDFunction, 0, onTypeUse, tok, tokenBytes, line, col)
	}

	line, col, err := lex(skipTokens(1, parser), []byte(source))
	if err != nil {
		err = &FormatError{Line: line, Col: col, cause: err}
	}
	return err
}

func TestTypeUseParser_ErrorContext(t *testing.T) {
	p := typeUseParser{currentParamField: 3}
	tests := []struct {
		source   string
		pos      parserPosition
		expected string
	}{
		{source: "initial", pos: positionInitial, expected: ""},
		{source: "param", pos: positionParam, expected: ".param[3]"},
		{source: "result", pos: positionResult, expected: ".result"},
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
