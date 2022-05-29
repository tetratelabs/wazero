package internal

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
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
		{tokenID, "ID"},
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

func TestStripDollar(t *testing.T) {
	require.Equal(t, []byte{'1'}, stripDollar([]byte{'$', '1'}))
}
