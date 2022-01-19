package text

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenType_String(t *testing.T) {
	tests := []struct {
		input    tokenType
		expected string
	}{
		{tokenKeyword, "keyword"},
		{tokenUN, "uN"},
		{tokenSN, "sN"},
		{tokenFN, "fN"},
		{tokenString, "string"},
		{tokenID, "id"},
		{tokenLParen, "("},
		{tokenRParen, ")"},
		{tokenReserved, "reserved"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.String())
		})
	}
}
