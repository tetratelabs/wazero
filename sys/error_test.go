package sys

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

type notExitError struct {
	exitCode uint32
}

func (e *notExitError) Error() string {
	return "not exit error"
}

func TestIs(t *testing.T) {
	err := NewExitError(2)
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
			name:    "different exit code",
			target:  NewExitError(1),
			matches: false,
		},
		{
			name: "different type",
			target: &notExitError{
				exitCode: 2,
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
		err := NewExitError(ExitCodeDeadlineExceeded)
		require.Equal(t, ExitCodeDeadlineExceeded, err.ExitCode())
		require.EqualError(t, err, "module closed with context deadline exceeded")
	})
	t.Run("cancel", func(t *testing.T) {
		err := NewExitError(ExitCodeContextCanceled)
		require.Equal(t, ExitCodeContextCanceled, err.ExitCode())
		require.EqualError(t, err, "module closed with context canceled")
	})
	t.Run("normal", func(t *testing.T) {
		err := NewExitError(123)
		require.Equal(t, uint32(123), err.ExitCode())
		require.EqualError(t, err, "module closed with exit_code(123)")
	})
}
