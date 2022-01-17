package wat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIndex(t *testing.T) {
	tests := []struct {
		name, input string
		expected    *index
	}{
		{"numeric", "(1)", &index{numeric: 0x1, line: 1, col: 2}},
		{"symbolic", "($v_v)", &index{ID: "v_v", line: 1, col: 2}},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ip := &indexParser{}

			var idx *index
			_, _, err := lex(eatFirstToken(ip.beginParsingIndex(func(i *index) {
				idx = i
			})), []byte(tc.input))
			require.NoError(t, err)
			require.Equal(t, tc.expected, idx)
		})
	}
}

func TestParseIndex_Errors(t *testing.T) {
	tests := []struct {
		name, input, expectedErr string
	}{
		{"nada", "()", "missing index"},
		{"numeric out of range", "(4294967296)", "index outside range of uint32: 4294967296"},
		{"double numeric", "(1 2)", "redundant index"},
		{"double ID", "($v_v $v_2)", "redundant index"},
		{"both", "($v_v 1)", "redundant index"},
		{"invalid", "(ice)", "unexpected keyword: ice"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ip := &indexParser{}

			var idx *index
			_, _, err := lex(eatFirstToken(ip.beginParsingIndex(func(i *index) {
				idx = i
			})), []byte(tc.input))
			require.EqualError(t, err, tc.expectedErr)
			require.Nil(t, idx)
		})
	}
}

// eatFirstToken is a hack because lex tracks parens, but beginParsingIndex starts after a field name
func eatFirstToken(parser tokenParser) tokenParser {
	ateFirstToken := false
	eatFirstTokenParser := func(tok tokenType, tokenBytes []byte, line, col uint32) error {
		if !ateFirstToken {
			ateFirstToken = true
			return nil
		}
		return parser(tok, tokenBytes, line, col)
	}
	return eatFirstTokenParser
}
