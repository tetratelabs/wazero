package internal

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestFormatError_Error(t *testing.T) {
	t.Run("with context", func(t *testing.T) {
		require.EqualError(t, &FormatError{
			Line:    1,
			Col:     2,
			Context: "start",
			cause:   errors.New("invalid token"),
		}, "1:2: invalid token in start")
	})
	t.Run("no context", func(t *testing.T) {
		require.EqualError(t, &FormatError{
			Line:  1,
			Col:   2,
			cause: errors.New("invalid token"),
		}, "1:2: invalid token")
	})
}

func TestFormatError_Unwrap(t *testing.T) {
	t.Run("cause", func(t *testing.T) {
		cause := errors.New("invalid token")
		formatErr := &FormatError{
			Line:    1,
			Col:     2,
			Context: "start",
			cause:   cause,
		}
		require.Equal(t, cause, formatErr.Unwrap())
	})
	t.Run("no cause", func(t *testing.T) {
		formatErr := &FormatError{
			Line:    1,
			Col:     2,
			Context: "start",
		}
		require.Nil(t, formatErr.Unwrap())
	})
}

func TestUnexpectedToken(t *testing.T) {
	tests := []struct {
		input      tokenType
		inputBytes []byte
		expected   string
	}{
		{tokenKeyword, []byte{'a'}, "unexpected keyword: a"},
		{tokenUN, []byte{'1'}, "unexpected uN: 1"},
		{tokenSN, []byte{'-', '1'}, "unexpected sN: -1"},
		{tokenFN, []byte{'1', '.', '1'}, "unexpected fN: 1.1"},
		{tokenString, []byte{'"', 'a', '"'}, "unexpected string: \"a\""},
		{tokenID, []byte{'$', 'a'}, "unexpected ID: $a"},
		{tokenLParen, []byte{'('}, "unexpected '('"},
		{tokenRParen, []byte{')'}, "unexpected ')'"},
		{tokenReserved, []byte{'0', '$'}, "unexpected reserved: 0$"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.input.String(), func(t *testing.T) {
			require.Equal(t, tc.expected, unexpectedToken(tc.input, tc.inputBytes).Error())
		})
	}
}

func TestUnhandledSection(t *testing.T) {
	require.Equal(t, "BUG: unhandled function", unhandledSection(wasm.SectionIDFunction).Error())
}

func TestUnexpectedFieldName(t *testing.T) {
	require.Equal(t, "unexpected field: moodule", unexpectedFieldName([]byte("moodule")).Error())
}

func TestExpectedField(t *testing.T) {
	require.Equal(t, "expected field, but parsed sN", expectedField(tokenSN).Error())
}
