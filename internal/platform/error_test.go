package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUnwrapOSError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected syscall.Errno
	}{
		{
			name:     "LinkError ErrInvalid",
			input:    &os.LinkError{Err: fs.ErrInvalid},
			expected: syscall.EINVAL,
		},
		{
			name:     "PathError ErrInvalid",
			input:    &os.PathError{Err: fs.ErrInvalid},
			expected: syscall.EINVAL,
		},
		{
			name:     "SyscallError ErrInvalid",
			input:    &os.SyscallError{Err: fs.ErrInvalid},
			expected: syscall.EINVAL,
		},
		{
			name:     "PathError ErrPermission",
			input:    &os.PathError{Err: os.ErrPermission},
			expected: syscall.EPERM,
		},
		{
			name:     "PathError ErrExist",
			input:    &os.PathError{Err: os.ErrExist},
			expected: syscall.EEXIST,
		},
		{
			name:     "PathError syscall.ErrnotExist",
			input:    &os.PathError{Err: os.ErrNotExist},
			expected: syscall.ENOENT,
		},
		{
			name:     "PathError ErrClosed",
			input:    &os.PathError{Err: os.ErrClosed},
			expected: syscall.EBADF,
		},
		{
			name:     "PathError unknown == syscall.EIO",
			input:    &os.PathError{Err: errors.New("ice cream")},
			expected: syscall.EIO,
		},
		{
			name:     "unknown == syscall.EIO",
			input:    errors.New("ice cream"),
			expected: syscall.EIO,
		},
		{
			name:     "very wrapped unknown == syscall.EIO",
			input:    fmt.Errorf("%w", fmt.Errorf("%w", fmt.Errorf("%w", errors.New("ice cream")))),
			expected: syscall.EIO,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			errno := UnwrapOSError(tc.input)
			require.EqualErrno(t, tc.expected, errno)
		})
	}

	t.Run("nil", func(t *testing.T) {
		require.Nil(t, UnwrapOSError(nil))
	})
}
