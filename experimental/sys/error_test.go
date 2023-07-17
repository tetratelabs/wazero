package sys

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
)

func TestUnwrapOSError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected Errno
	}{
		{
			name:     "io.EOF is not an error",
			input:    io.EOF,
			expected: 0,
		},
		{
			name:     "LinkError ErrInvalid",
			input:    &os.LinkError{Err: fs.ErrInvalid},
			expected: EINVAL,
		},
		{
			name:     "PathError ErrInvalid",
			input:    &os.PathError{Err: fs.ErrInvalid},
			expected: EINVAL,
		},
		{
			name:     "SyscallError ErrInvalid",
			input:    &os.SyscallError{Err: fs.ErrInvalid},
			expected: EINVAL,
		},
		{
			name:     "PathError ErrPermission",
			input:    &os.PathError{Err: os.ErrPermission},
			expected: EPERM,
		},
		{
			name:     "PathError ErrExist",
			input:    &os.PathError{Err: os.ErrExist},
			expected: EEXIST,
		},
		{
			name:     "PathError ErrnotExist",
			input:    &os.PathError{Err: os.ErrNotExist},
			expected: ENOENT,
		},
		{
			name:     "PathError ErrClosed",
			input:    &os.PathError{Err: os.ErrClosed},
			expected: EBADF,
		},
		{
			name:     "PathError unknown == EIO",
			input:    &os.PathError{Err: errors.New("ice cream")},
			expected: EIO,
		},
		{
			name:     "unknown == EIO",
			input:    errors.New("ice cream"),
			expected: EIO,
		},
		{
			name:     "very wrapped unknown == EIO",
			input:    fmt.Errorf("%w", fmt.Errorf("%w", fmt.Errorf("%w", errors.New("ice cream")))),
			expected: EIO,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			// don't use require package as that introduces a package cycle
			if want, have := tc.expected, UnwrapOSError(tc.input); have != want {
				t.Fatalf("unexpected errno: %v != %v", have, want)
			}
		})
	}

	t.Run("nil -> zero", func(t *testing.T) {
		// don't use require package as that introduces a package cycle
		if want, have := Errno(0), UnwrapOSError(nil); have != want {
			t.Fatalf("unexpected errno: %v != %v", have, want)
		}
	})
}
