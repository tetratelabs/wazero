package internalwasm

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/cstring"
)

func TestSystemContext_Defaults(t *testing.T) {
	sys, err := NewSystemContext()
	require.Nil(t, err)

	require.Equal(t, cstring.EmptyNullTerminatedStrings, sys.Args)
	require.Equal(t, cstring.EmptyNullTerminatedStrings, sys.Environ)
	require.Equal(t, os.Stdin, sys.Stdin)
	require.Equal(t, os.Stdout, sys.Stdout)
	require.Equal(t, os.Stderr, sys.Stderr)
	require.Empty(t, sys.OpenedFiles)
}

func TestSystemContext_WithArgs(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		sys, err := NewSystemContext()
		require.Nil(t, err)

		err = sys.WithArgs("a", "bc")
		require.NoError(t, err)

		require.Equal(t, &cstring.NullTerminatedStrings{
			NullTerminatedValues: [][]byte{
				{'a', 0},
				{'b', 'c', 0},
			},
			TotalBufSize: 5,
		}, sys.Args)
	})
	t.Run("error constructing args", func(t *testing.T) {
		sys, err := NewSystemContext()
		require.Nil(t, err)

		err = sys.WithArgs("\xff\xfe\xfd", "foo", "bar")
		require.EqualError(t, err, "arg[0] is not a valid UTF-8 string")
	})
}

func TestSystemContext_WithEnviron(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		sys, err := NewSystemContext()
		require.Nil(t, err)

		err = sys.WithEnviron("a=b", "b=cd")
		require.NoError(t, err)

		require.Equal(t, &cstring.NullTerminatedStrings{
			NullTerminatedValues: [][]byte{
				{'a', '=', 'b', 0},
				{'b', '=', 'c', 'd', 0},
			},
			TotalBufSize: 9,
		}, sys.Environ)
	})

	errorTests := []struct {
		name         string
		environ      string
		errorMessage string
	}{
		{name: "error invalid utf-8",
			environ:      "non_utf8=\xff\xfe\xfd",
			errorMessage: "environ[0] is not a valid UTF-8 string"},
		{name: "error not '='-joined pair",
			environ:      "no_equal_pair",
			errorMessage: "environ[0] is not joined with '='"},
	}
	for _, tt := range errorTests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sys, err := NewSystemContext()
			require.Nil(t, err)

			err = sys.WithEnviron(tc.environ)
			require.EqualError(t, err, tc.errorMessage)
		})
	}
}
