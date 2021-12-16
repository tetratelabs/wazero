package wat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenType_String(t *testing.T) {
	tests := []struct {
		input    tokenType
		expected string
	}{
		{tokenKeyword, "tokenKeyword"},
		{tokenUN, "tokenUN"},
		{tokenSN, "tokenSN"},
		{tokenFN, "tokenFN"},
		{tokenString, "tokenString"},
		{tokenID, "tokenID"},
		{tokenLParen, "tokenLParen"},
		{tokenRParen, "tokenRParen"},
		{tokenReserved, "tokenReserved"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.String())
		})
	}
}
