package internal

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var (
	f32, f64, i32, i64 = wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64
	i32_v              = &wasm.FunctionType{Params: []wasm.ValueType{i32}}
	v_i32              = &wasm.FunctionType{Results: []wasm.ValueType{i32}}
	v_i32i64           = &wasm.FunctionType{Results: []wasm.ValueType{i32, i64}}
	f32_i32            = &wasm.FunctionType{Params: []wasm.ValueType{f32}, Results: []wasm.ValueType{i32}}
	i64_i64            = &wasm.FunctionType{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i64}}
	i32i64_v           = &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}}
	i32i32_i32         = &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	i32i64_i32         = &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32}}
	i32i32i32i32_i32   = &wasm.FunctionType{
		Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}
	i32i32i32i32i32i64i64i32i32_i32 = &wasm.FunctionType{
		Params:  []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
		Results: []wasm.ValueType{i32},
	}
)

func TestTypeParser(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expected   *wasm.FunctionType
		expectedID string
	}{
		{
			name:     "empty",
			input:    "(type (func))",
			expected: &wasm.FunctionType{},
		},
		{
			name:       "empty - ID",
			input:      "(type $v_v (func))",
			expected:   &wasm.FunctionType{},
			expectedID: "v_v",
		},
		{
			name:     "param no result",
			input:    "(type (func (param i32)))",
			expected: i32_v,
		},
		{
			name:       "param no result - ID",
			input:      "(type $i32_v (func (param i32)))",
			expected:   i32_v,
			expectedID: "i32_v",
		},
		{
			name:     "result no param",
			input:    "(type (func (result i32)))",
			expected: v_i32,
		},
		{
			name:       "result no param - ID",
			input:      "(type $v_i32 (func (result i32)))",
			expected:   v_i32,
			expectedID: "v_i32",
		},
		{
			name:     "results no param",
			input:    "(type (func (result i32) (result i64)))",
			expected: v_i32i64,
		},
		{
			name:     "mixed param no result",
			input:    "(type (func (param i32) (param i64)))",
			expected: i32i64_v,
		},
		{
			name:       "mixed param no result - ID",
			input:      "(type $i32i64_v (func (param i32) (param i64)))",
			expected:   i32i64_v,
			expectedID: "i32i64_v",
		},
		{
			name:     "mixed param result",
			input:    "(type (func (param i32) (param i64) (result i32)))",
			expected: i32i64_i32,
		},
		{
			name:       "mixed param result - ID",
			input:      "(type $i32i64_i32 (func (param i32) (param i64) (result i32)))",
			expected:   i32i64_i32,
			expectedID: "i32i64_i32",
		},
		{
			name:     "abbreviated param result",
			input:    "(type (func (param i32 i64) (result i32)))",
			expected: i32i64_i32,
		},
		{
			name:     "mixed param abbreviation", // Verifies we can handle less param fields than param types
			input:    "(type (func (param i32 i32) (param i32) (param i64) (param f32)))",
			expected: &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i64, f32}},
		},

		// Below are changes to test/core/br.wast from the commit that added "multi-value" support.
		// See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c

		{
			name:     "multi-value - v_i64f32 abbreviated",
			input:    "(type (func (result i64 f32)))",
			expected: &wasm.FunctionType{Results: []wasm.ValueType{i64, f32}},
		},
		{
			name:     "multi-value - i32i64_f32f64 abbreviated",
			input:    "(type (func (param i32 i64) (result f32 f64)))",
			expected: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
		},
		{
			name:     "multi-value - v_i64f32",
			input:    "(type (func (result i64) (result f32)))",
			expected: &wasm.FunctionType{Results: []wasm.ValueType{i64, f32}},
		},
		{
			name:     "multi-value - i32i64_f32f64",
			input:    "(type (func (param i32) (param i64) (result f32) (result f64)))",
			expected: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
		},
		{
			name:     "multi-value - i32i64_f32f64 named",
			input:    "(type (func (param $x i32) (param $y i64) (result f32) (result f64)))",
			expected: &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}},
		},
		{
			name:     "multi-value - i64i64f32_f32i32 results abbreviated in groups",
			input:    "(type (func (result i64 i64 f32) (result f32 i32)))",
			expected: &wasm.FunctionType{Results: []wasm.ValueType{i64, i64, f32, f32, i32}},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 params and results abbreviated in groups",
			input: "(type (func (param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32)))",
			expected: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 abbreviated in groups",
			input: "(type (func (param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32)))",
			expected: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - i32i32i64i32_f32f64f64i32 abbreviated in groups",
			input: "(type (func (param i32 i32) (param i64 i32) (result f32 f64) (result f64 i32)))",
			expected: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
		{
			name:  "multi-value - empty abbreviated results",
			input: "(type (func (result) (result) (result i64 i64) (result) (result f32) (result)))",
			// Abbreviations have min length zero, which implies no-op results are ok.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A2
			expected: &wasm.FunctionType{Results: []wasm.ValueType{i64, i64, f32}},
		},
		{
			name: "multi-value - empty abbreviated params and results",
			input: `(type (func
  (param i32 i32) (param i64 i32) (param) (param $x i32) (param)
  (result) (result f32 f64) (result f64 i32) (result)
))`,
			// Abbreviations have min length zero, which implies no-op results are ok.
			// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#abbreviations%E2%91%A2
			expected: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i64, i32, i32},
				Results: []wasm.ValueType{f32, f64, f64, i32},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			typeNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
				require.Equal(t, wasm.SectionIDType, sectionID)
				return 0
			})
			parsed, tp, err := parseFunctionType(wasm.Features20220419, typeNamespace, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, parsed)
			require.Equal(t, uint32(1), tp.typeNamespace.count)
			if tc.expectedID == "" {
				require.Zero(t, len(tp.typeNamespace.idToIdx), "expected no indices")
			} else {
				// Since the parser was initially empty, the expected index of the parsed type is 0
				require.Equal(t, map[string]wasm.Index{tc.expectedID: wasm.Index(0)}, tp.typeNamespace.idToIdx)
			}
		})
	}
}

func TestTypeParser_Errors(t *testing.T) {
	tests := []struct {
		name, input, expectedErr string
		enabledFeatures          wasm.Features
	}{
		{
			name:        "invalid token",
			input:       "(type \"0\")",
			expectedErr: "unexpected string: \"0\"",
		},
		{
			name:        "missing desc",
			input:       "(type)",
			expectedErr: "missing func field",
		},
		{
			name:        "not desc field",
			input:       "(type ($func))",
			expectedErr: "expected field, but parsed ID",
		},
		{
			name:        "wrong desc field",
			input:       "(type (funk))",
			expectedErr: "unexpected field: funk",
		},
		{
			name:        "second ID",
			input:       "(type $v_v $v_v func())",
			expectedErr: "redundant ID $v_v",
		},
		{
			name:        "func repeat",
			input:       "(type (func) (func))",
			expectedErr: "unexpected '('",
		},
		{
			name:        "func not field",
			input:       "(type (func ($param i64))",
			expectedErr: "unexpected ID: $param",
		},
		{
			name:        "func not param or result field",
			input:       "(type (func (type 0))",
			expectedErr: "unexpected field: type",
		},
		{
			name:        "param wrong type",
			input:       "(type (func (param i33)))",
			expectedErr: "unknown type: i33",
		},
		{
			name:        "param ID in abbreviation",
			input:       "(type (func (param $x i32 i64) )",
			expectedErr: "cannot assign IDs to parameters in abbreviated form",
		},
		{
			name:        "param second ID",
			input:       "(type (func (param $x $x i64) )",
			expectedErr: "redundant ID $x",
		},
		{
			name:        "param wrong end",
			input:       "(type (func (param i64 \"\")))",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "result has no ID",
			input:       "(type (func (result $x i64)))",
			expectedErr: "unexpected ID: $x",
		},
		{
			name:        "result wrong type",
			input:       "(type (func (result i33)))",
			expectedErr: "unknown type: i33",
		},
		{
			name:        "result abbreviated",
			input:       "(type (func (result i32 i64)))",
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:        "result twice",
			input:       "(type (func (result i32) (result i32)))",
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:            "result second wrong",
			input:           "(type (func (result i32) (result i33)))",
			enabledFeatures: wasm.Features20220419,
			expectedErr:     "unknown type: i33",
		},
		{
			name:            "result second redundant type wrong",
			input:           "(type (func (result i32) (result i32 i33)))",
			enabledFeatures: wasm.Features20220419,
			expectedErr:     "unknown type: i33",
		},
		{
			name:        "param after result",
			input:       "(type (func (result i32) (param i32)))",
			expectedErr: "param after result",
		},
		{
			name:        "result wrong end",
			input:       "(type (func (result i64 \"\")))",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "func has no ID",
			input:       "(type (func $v_v )))",
			expectedErr: "unexpected ID: $v_v",
		},
		{
			name:        "func invalid token",
			input:       "(type (func \"0\")))",
			expectedErr: "unexpected string: \"0\"",
		},
		{
			name:        "wrong end",
			input:       "(type (func) \"\")",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "wrong end - after func",
			input:       "(type (func (param i32) \"\")))",
			expectedErr: "unexpected string: \"\"",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = wasm.Features20191205
			}
			typeNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
				require.Equal(t, wasm.SectionIDType, sectionID)
				return 0
			})
			parsed, _, err := parseFunctionType(enabledFeatures, typeNamespace, tc.input)
			require.EqualError(t, err, tc.expectedErr)
			require.Nil(t, parsed)
		})
	}

	t.Run("duplicate ID", func(t *testing.T) {
		typeNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
			require.Equal(t, wasm.SectionIDType, sectionID)
			return 0
		})
		_, err := typeNamespace.setID([]byte("$v_v"))
		require.NoError(t, err)
		typeNamespace.count++

		parsed, _, err := parseFunctionType(wasm.Features20191205, typeNamespace, "(type $v_v (func))")
		require.EqualError(t, err, "duplicate ID $v_v")
		require.Nil(t, parsed)
	})
}

func parseFunctionType(
	enabledFeatures wasm.Features,
	typeNamespace *indexNamespace,
	input string,
) (*wasm.FunctionType, *typeParser, error) {
	var parsed *wasm.FunctionType
	var setFunc onType = func(ft *wasm.FunctionType) tokenParser {
		parsed = ft
		return parseErr
	}
	tp := newTypeParser(enabledFeatures, typeNamespace, setFunc)
	// typeParser starts after the '(type', so we need to eat it first!
	_, _, err := lex(skipTokens(2, tp.begin), []byte(input))
	return parsed, tp, err
}

func TestParseValueType(t *testing.T) {
	tests := []struct {
		input    string
		expected wasm.ValueType
	}{
		{input: "i32", expected: wasm.ValueTypeI32},
		{input: "i64", expected: wasm.ValueTypeI64},
		{input: "f32", expected: wasm.ValueTypeF32},
		{input: "f64", expected: wasm.ValueTypeF64},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.input, func(t *testing.T) {
			m, err := parseValueType([]byte(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, m)
		})
	}
	t.Run("unknown type", func(t *testing.T) {
		_, err := parseValueType([]byte("f65"))
		require.EqualError(t, err, "unknown type: f65")
	})
}

func TestTypeParser_ErrorContext(t *testing.T) {
	p := typeParser{currentField: 3, currentType: &wasm.FunctionType{}}
	tests := []struct {
		input    string
		pos      parserPosition
		expected string
	}{
		{input: "initial", pos: positionInitial, expected: ""},
		{input: "func", pos: positionFunc, expected: ".func"},
		{input: "param", pos: positionParam, expected: ".func.param[3]"},
		{input: "result", pos: positionResult, expected: ".func.result[3]"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.input, func(t *testing.T) {
			p.pos = tc.pos
			require.Equal(t, tc.expected, p.errorContext())
		})
	}
}
