package text

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestIdContext_SetId(t *testing.T) {
	ctx := idContext{}
	t.Run("set when empty", func(t *testing.T) {
		id, err := ctx.setID([]byte("$x"), wasm.Index(0))
		require.NoError(t, err)
		require.Equal(t, "x", id) // strips "$" to be like the name section
		require.Equal(t, idContext{"x": wasm.Index(0)}, ctx)
	})
	t.Run("set when exists fails", func(t *testing.T) {
		_, err := ctx.setID([]byte("$x"), wasm.Index(0))
		require.EqualError(t, err, "duplicate ID $x")        // keeps the original $ prefix
		require.Equal(t, idContext{"x": wasm.Index(0)}, ctx) // no change
	})
}

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
