package sys

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

type notExitError struct {
	moduleName string
	exitCode   uint32
}

func (e *notExitError) Error() string {
	return "not exit error"
}

func TestIs(t *testing.T) {
	err := NewExitError("some module", 2)
	tests := []struct {
		name    string
		target  error
		matches bool
	}{
		{
			name:    "same object",
			target:  err,
			matches: true,
		},
		{
			name:    "same content",
			target:  NewExitError("some module", 2),
			matches: true,
		},
		{
			name:    "different module name",
			target:  NewExitError("not some module", 2),
			matches: false,
		},
		{
			name:    "different exit code",
			target:  NewExitError("some module", 0),
			matches: false,
		},
		{
			name: "different type",
			target: &notExitError{
				moduleName: "some module",
				exitCode:   2,
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			matches := errors.Is(err, tc.target)
			require.Equal(t, tc.matches, matches)
		})
	}
}

func TestExitError_Error(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		err := NewExitError("foo", ExitCodeDeadlineExceeded)
		require.Equal(t, ExitCodeDeadlineExceeded, err.ExitCode())
		require.EqualError(t, err, "module \"foo\" closed with context deadline exceeded")
	})
	t.Run("cancel", func(t *testing.T) {
		err := NewExitError("foo", ExitCodeContextCanceled)
		require.Equal(t, ExitCodeContextCanceled, err.ExitCode())
		require.EqualError(t, err, "module \"foo\" closed with context canceled")
	})
	t.Run("normal", func(t *testing.T) {
		err := NewExitError("foo", 123)
		require.Equal(t, uint32(123), err.ExitCode())
		require.EqualError(t, err, "module \"foo\" closed with exit_code(123)")
	})
}
