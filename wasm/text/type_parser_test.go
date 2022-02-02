package text

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

var (
	f32, i32, i64                   = wasm.ValueTypeF32, wasm.ValueTypeI32, wasm.ValueTypeI64
	i32_v                           = &wasm.FunctionType{Params: []wasm.ValueType{i32}}
	v_i32                           = &wasm.FunctionType{Results: []wasm.ValueType{i32}}
	i32i64_v                        = &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}}
	i32i32_i32                      = &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	i32i64_i32                      = &wasm.FunctionType{Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{i32}}
	i32i32i32i32_i32                = &wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}
	i32i32i32i32i32i64i64i32i32_i32 = &wasm.FunctionType{
		Params:  []wasm.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
		Results: []wasm.ValueType{i32},
	}
)

func TestTypeParser(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *wasm.FunctionType
		expectedErr string
	}{
		{
			name:     "empty",
			input:    "(type (func))",
			expected: &wasm.FunctionType{},
		},
		{
			name:        "empty - ID",
			input:       "(type $v_v (func))",
			expected:    &wasm.FunctionType{},
			expectedErr: "v_v",
		},
		{
			name:     "param no result",
			input:    "(type (func (param i32)))",
			expected: i32_v,
		},
		{
			name:        "param no result - ID",
			input:       "(type $i32_v (func (param i32)))",
			expected:    i32_v,
			expectedErr: "i32_v",
		},
		{
			name:     "result no param",
			input:    "(type (func (result i32)))",
			expected: v_i32,
		},
		{
			name:        "result no param - ID",
			input:       "(type $v_i32 (func (result i32)))",
			expected:    v_i32,
			expectedErr: "v_i32",
		},
		{
			name:     "mixed param no result",
			input:    "(type (func (param i32) (param i64)))",
			expected: i32i64_v,
		},
		{
			name:        "mixed param no result - ID",
			input:       "(type $i32i64_v (func (param i32) (param i64)))",
			expected:    i32i64_v,
			expectedErr: "i32i64_v",
		},
		{
			name:     "mixed param result",
			input:    "(type (func (param i32) (param i64) (result i32)))",
			expected: i32i64_i32,
		},
		{
			name:        "mixed param result - ID",
			input:       "(type $i32i64_i32 (func (param i32) (param i64) (result i32)))",
			expected:    i32i64_i32,
			expectedErr: "i32i64_i32",
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
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			namespace := newIndexNamespace()
			parsed, tp, err := parseFunctionType(namespace, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, parsed)
			require.Equal(t, uint32(1), tp.typeNamespace.count)
			if tc.expectedErr == "" {
				require.Empty(t, tp.typeNamespace.idToIdx)
			} else {
				// Since the parser was initially empty, the expected index of the parsed type is 0
				require.Equal(t, map[string]wasm.Index{tc.expectedErr: wasm.Index(0)}, tp.typeNamespace.idToIdx)
			}
		})
	}
}

func TestTypeParser_Errors(t *testing.T) {
	tests := []struct{ name, input, expectedErr string }{
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
			name:        "param missing type",
			input:       "(type (func (param))",
			expectedErr: "expected a type",
		},
		{
			name:        "param wrong type",
			input:       "(type (func (param i33))",
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
			input:       "(type (func (param i64 \"\"))",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "result has no ID",
			input:       "(type (func (result $x i64) )",
			expectedErr: "unexpected ID: $x",
		},
		{
			name:        "result missing type",
			input:       "(type (func (result))",
			expectedErr: "expected a type",
		},
		{
			name:        "result wrong type",
			input:       "(type (func (result i33))",
			expectedErr: "unknown type: i33",
		},
		{
			name:        "result wrong end",
			input:       "(type (func (result i64 \"\"))",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "func has no ID",
			input:       "(type (func $v_v ))",
			expectedErr: "unexpected ID: $v_v",
		},
		{
			name:        "func invalid token",
			input:       "(type (func \"0\"))",
			expectedErr: "unexpected string: \"0\"",
		},
		{
			name:        "wrong end",
			input:       "(type (func) \"\")",
			expectedErr: "unexpected string: \"\"",
		},
		{
			name:        "wrong end - after func",
			input:       "(type (func (param i32) \"\"))",
			expectedErr: "unexpected string: \"\"",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			parsed, _, err := parseFunctionType(newIndexNamespace(), tc.input)
			require.EqualError(t, err, tc.expectedErr)
			require.Nil(t, parsed)
		})
	}

	t.Run("duplicate ID", func(t *testing.T) {
		funcIndexNamespace := newIndexNamespace()
		_, err := funcIndexNamespace.setID([]byte("$v_v"))
		require.NoError(t, err)
		funcIndexNamespace.count++

		parsed, _, err := parseFunctionType(funcIndexNamespace, "(type $v_v (func))")
		require.EqualError(t, err, "duplicate ID $v_v")
		require.Nil(t, parsed)
	})
}

func parseFunctionType(namespace *indexNamespace, input string) (*wasm.FunctionType, *typeParser, error) {
	var parsed *wasm.FunctionType
	var setFunc onType = func(ft *wasm.FunctionType) tokenParser {
		parsed = ft
		return parseErr
	}
	tp := newTypeParser(namespace, setFunc)
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

func TestParseResultType(t *testing.T) {
	tests := []struct {
		name        string
		tokenBytes  string
		expected    []wasm.ValueType
		expectedErr string
	}{
		{
			name:       "no value",
			tokenBytes: "i32",
			expected:   []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			name:        "invalid token",
			tokenBytes:  "i33",
			expectedErr: "unknown type: i33",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			rt, err := parseResultType([]byte(tc.tokenBytes))
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, rt)
			}
		})
	}
}

func TestTypeParser_ErrorContext(t *testing.T) {
	p := typeParser{currentParamField: 3}
	tests := []struct {
		input    string
		pos      parserPosition
		expected string
	}{
		{input: "initial", pos: positionInitial, expected: ""},
		{input: "func", pos: positionFunc, expected: ".func"},
		{input: "param", pos: positionParam, expected: ".func.param[3]"},
		{input: "result", pos: positionResult, expected: ".func.result"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.input, func(t *testing.T) {
			p.pos = tc.pos
			require.Equal(t, tc.expected, p.errorContext())
		})
	}
}
