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

	expectedOnTypeUsePosition onTypeUsePosition
	expectedOnTypeUseToken    tokenType
	expectedTrailingTokens    []tokenType
}

func TestTypeUseParser(t *testing.T) {
	f32, i32, i64 := wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeI64
	i32_v := &wasm.FunctionType{Params: []wasm.ValueType{i32}}
	v_i32 := &wasm.FunctionType{Results: []wasm.ValueType{i32}}
	i32_i64_v := &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}}
	i32_i64_i32 := &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32}}

	tests := []*typeUseParserTest{
		{
			name:                "empty",
			source:              "()",
			expectedInlinedType: emptyFunctionType,
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
			expectedInlinedType: i32_i64_v,
		},
		{
			name:                "mixed param no result - ID",
			source:              "((param $x i32) (param $y i64))",
			expectedInlinedType: i32_i64_v,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "mixed param result",
			source:              "((param i32) (param i64) (result i32))",
			expectedInlinedType: i32_i64_i32,
		},
		{
			name:                "mixed param result - ID",
			source:              "((param $x i32) (param $y i64) (result i32))",
			expectedInlinedType: i32_i64_i32,
			expectedParamNames:  wasm.NameMap{&wasm.NameAssoc{Index: 0, Name: "x"}, &wasm.NameAssoc{Index: 1, Name: "y"}},
		},
		{
			name:                "abbreviated param result",
			source:              "((param i32 i64) (result i32))",
			expectedInlinedType: i32_i64_i32,
		},
		{
			name:                "mixed param abbreviation", // Verifies we can handle less param fields than param types
			source:              "((param i32 i32) (param i32) (param i64) (param f32))",
			expectedInlinedType: &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
		},
	}

	runTypeUseParserTests(t, tests, func() (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
		return tp, func(t *testing.T) {
			require.Nil(t, tp.typeNamespace.unresolvedIndices)
		}
	})

	tests = []*typeUseParserTest{
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
			expectedInlinedType: i32_i64_i32,
		},
		{
			name:                "unresolved type - ID - match",
			source:              "((type $i32_i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx:     0,
			expectedInlinedType: i32_i64_i32,
		},
	}
	runTypeUseParserTests(t, tests, func() (*typeUseParser, func(t *testing.T)) {
		tp := newTypeUseParser(&wasm.Module{}, newIndexNamespace())
		return tp, func(t *testing.T) {
			require.NotNil(t, tp.typeNamespace.unresolvedIndices)
		}
	})

	tests = []*typeUseParserTest{
		{
			name:            "reuse existing result type",
			source:          "((result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "reuse existing nullary type",
			source:          "()",
			expectedTypeIdx: 1,
		},
		{
			name:            "reuse existing param type",
			source:          "((param i32))",
			expectedTypeIdx: 2,
		},

		{
			name:            "reuse existing param and result type",
			source:          "((param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "found result type - index",
			source:          "((type 0))",
			expectedTypeIdx: 0,
		},
		{
			name:            "found result type - ID",
			source:          "((type $v_i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "found result type - ID - matched",
			source:          "((type $v_i32) (result i32))",
			expectedTypeIdx: 0,
		},
		{
			name:            "found nullary type - index",
			source:          "((type 1))",
			expectedTypeIdx: 1,
		},
		{
			name:            "found nullary type - ID",
			source:          "((type $v_v))",
			expectedTypeIdx: 1,
		},
		{
			name:            "found param type - index",
			source:          "((type 2))",
			expectedTypeIdx: 2,
		},
		{
			name:            "found param type - ID",
			source:          "((type $i32_v))",
			expectedTypeIdx: 2,
		},
		{
			name:            "found param type - ID - matched",
			source:          "((type $i32_v) (param i32))",
			expectedTypeIdx: 2,
		},
		{
			name:            "found param and result type - index",
			source:          "((type 3))",
			expectedTypeIdx: 3,
		},
		{
			name:            "found param and result type - ID",
			source:          "((type $i32_i64_i32))",
			expectedTypeIdx: 3,
		},
		{
			name:            "found param type - ID - matched",
			source:          "((type $i32_i64_i32) (param i32 i64) (result i32))",
			expectedTypeIdx: 3,
		},
	}
	runTypeUseParserTests(t, tests, func() (*typeUseParser, func(t *testing.T)) {
		typeNamespace := newIndexNamespace()

		// Add types to cover the main ways types uses are declared
		module := &wasm.Module{TypeSection: []*wasm.FunctionType{v_i32, emptyFunctionType, i32_v, i32_i64_i32}}
		_, err := typeNamespace.setID([]byte("$v_i32"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$v_v"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$i32_v"))
		require.NoError(t, err)
		typeNamespace.count++

		_, err = typeNamespace.setID([]byte("$i32_i64_i32"))
		require.NoError(t, err)
		typeNamespace.count++

		tp := newTypeUseParser(module, typeNamespace)
		return tp, func(t *testing.T) {
			require.Nil(t, tp.typeNamespace.unresolvedIndices)
			require.Nil(t, tp.inlinedTypes)
		}
	})
}

// To prevent having to maintain a lot of tests to cover the necessary dimensions, this generates combinations that
// can happen in a type use.
// Ex. (func ) (func (local i32)) and (func nop) are all empty type uses, and we need to hit all three cases
func runTypeUseParserTests(t *testing.T, tests []*typeUseParserTest, tf testFunc) {
	moreTests := make([]*typeUseParserTest, 0, len(tests)*2)
	for _, tt := range tests {
		tt.expectedOnTypeUsePosition = onTypeUseEndField
		tt.expectedOnTypeUseToken = tokenRParen // at the end of the field ')'
		tt.expectedTrailingTokens = nil

		kt := *tt // copy
		kt.name = fmt.Sprintf("%s - trailing keyword", tt.name)
		kt.source = fmt.Sprintf("%s nop)", tt.source[:len(tt.source)-1])
		kt.expectedOnTypeUsePosition = onTypeUseUnhandledToken
		kt.expectedOnTypeUseToken = tokenKeyword // at 'nop' and ')' remains
		kt.expectedTrailingTokens = []tokenType{tokenRParen}
		moreTests = append(moreTests, &kt)

		ft := *tt // copy
		ft.name = fmt.Sprintf("%s - trailing field", tt.name)
		ft.source = fmt.Sprintf("%s (nop))", tt.source[:len(tt.source)-1])
		// Two outcomes, we've reached a field not named "type", "param" or "result" or we completed "result"
		if strings.Contains(tt.source, "result") {
			ft.expectedOnTypeUsePosition = onTypeUseUnhandledToken
			ft.expectedOnTypeUseToken = tokenLParen // at '(' and 'nop))' remain
			ft.expectedTrailingTokens = []tokenType{tokenKeyword, tokenRParen, tokenRParen}
		} else {
			ft.expectedOnTypeUsePosition = onTypeUseUnhandledField
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
			var setTypeUse onTypeUse = func(typeIdx wasm.Index, paramNames wasm.NameMap, pos onTypeUsePosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
				parsedTypeIdx = typeIdx
				parsedParamNames = paramNames
				require.Equal(t, tc.expectedOnTypeUsePosition, pos)
				require.Equal(t, tc.expectedOnTypeUseToken, tok)
				return p.parse, nil
			}

			tp, test := tf()
			require.NoError(t, parseTypeUse(tp, tc.source, setTypeUse))
			require.Equal(t, tc.expectedTrailingTokens, p.tokenTypes)
			require.Equal(t, tc.expectedTypeIdx, parsedTypeIdx)
			require.Equal(t, tc.expectedParamNames, parsedParamNames)
			if tc.expectedInlinedType == nil {
				require.Empty(t, tp.inlinedTypes)
			} else {
				require.Equal(t, tc.expectedInlinedType, tp.inlinedTypes[0])
			}
			test(t)
		})
	}
}

type testFunc func() (*typeUseParser, func(t *testing.T))

func TestTypeUseParser_Errors(t *testing.T) {
	tests := []struct{ name, source, expectedErr string }{
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
			name:        "result wrong end",
			source:      `((result i64 ""))`,
			expectedErr: "1:14: unexpected string: \"\"",
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

var failOnTypeUse onTypeUse = func(typeIdx wasm.Index, paramNames wasm.NameMap, pos onTypeUsePosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
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
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.source, func(t *testing.T) {
			p.pos = tc.pos
			require.Equal(t, tc.expected, p.errorContext())
		})
	}
}
