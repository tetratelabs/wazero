package wat

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
