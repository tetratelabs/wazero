package wat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBindIndices(t *testing.T) {
	tests := []struct {
		name            string
		input, expected *module
	}{
		{
			name: "start imported function by name binds to numeric index",
			input: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{funcName: "$one", typeInlined: typeFuncEmpty},
					{funcName: "$two", typeInlined: typeFuncEmpty},
				},
				startFunction: &index{ID: "$two", line: 3, col: 9},
			},
			expected: &module{
				typeFuncs: []*typeFunc{typeFuncEmpty},
				importFuncs: []*importFunc{
					{funcName: "$one", typeInlined: typeFuncEmpty},
					{funcName: "$two", typeInlined: typeFuncEmpty},
				},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
		},
		{
			name: "start imported function numeric index left alone",
			input: &module{
				typeFuncs:     []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeInlined: typeFuncEmpty}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
			expected: &module{
				typeFuncs:     []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeInlined: typeFuncEmpty}},
				startFunction: &index{numeric: 0, line: 3, col: 9},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, tc.input)
		})
	}
}

func TestBindIndices_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       *module
		expectedErr string
	}{
		{
			name: "start points out of range",
			input: &module{
				typeFuncs:     []*typeFunc{typeFuncEmpty},
				importFuncs:   []*importFunc{{name: "hello", importIndex: 0, typeInlined: typeFuncEmpty}},
				startFunction: &index{numeric: 1, line: 3, col: 9},
			},
			expectedErr: "3:9: function index 1 is out of range [0..0] in module.start",
		},
		{
			name: "start points nowhere",
			input: &module{
				startFunction: &index{ID: "$main", line: 1, col: 16},
			},
			expectedErr: "1:16: unknown function name $main in module.start",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := bindIndices(tc.input)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
