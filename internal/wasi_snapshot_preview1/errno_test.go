package wasi_snapshot_preview1

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestToErrno(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected Errno
	}{
		{
			name:     "syscall.EBADF",
			input:    syscall.EBADF,
			expected: ErrnoBadf,
		},
		{
			name:     "syscall.EEXIST",
			input:    syscall.EEXIST,
			expected: ErrnoExist,
		},
		{
			name:     "syscall.EINVAL",
			input:    syscall.EINVAL,
			expected: ErrnoInval,
		},
		{
			name:     "syscall.EIO",
			input:    syscall.EIO,
			expected: ErrnoIo,
		},
		{
			name:     "syscall.EISDIR",
			input:    syscall.EISDIR,
			expected: ErrnoIsdir,
		},
		{
			name:     "syscall.ELOOP",
			input:    syscall.ELOOP,
			expected: ErrnoLoop,
		},
		{
			name:     "syscall.ENAMETOOLONG",
			input:    syscall.ENAMETOOLONG,
			expected: ErrnoNametoolong,
		},
		{
			name:     "syscall.ENOENT",
			input:    syscall.ENOENT,
			expected: ErrnoNoent,
		},
		{
			name:     "syscall.ENOSYS",
			input:    syscall.ENOSYS,
			expected: ErrnoNosys,
		},
		{
			name:     "syscall.ENOTDIR",
			input:    syscall.ENOTDIR,
			expected: ErrnoNotdir,
		},
		{
			name:     "syscall.ENOTEMPTY",
			input:    syscall.ENOTEMPTY,
			expected: ErrnoNotempty,
		},
		{
			name:     "syscall.ENOTSUP",
			input:    syscall.ENOTSUP,
			expected: ErrnoNotsup,
		},
		{
			name:     "syscall.EPERM",
			input:    syscall.EPERM,
			expected: ErrnoPerm,
		},
		{
			name:     "syscall.Errno unexpected == ErrnoIo",
			input:    syscall.Errno(0xfe),
			expected: ErrnoIo,
		},
		{
			name:     "PathError ErrInvalid",
			input:    &os.PathError{Err: fs.ErrInvalid},
			expected: ErrnoInval,
		},
		{
			name:     "PathError ErrPermission",
			input:    &os.PathError{Err: fs.ErrPermission},
			expected: ErrnoPerm,
		},
		{
			name:     "PathError ErrExist",
			input:    &os.PathError{Err: fs.ErrExist},
			expected: ErrnoExist,
		},
		{
			name:     "PathError ErrNotExist",
			input:    &os.PathError{Err: fs.ErrNotExist},
			expected: ErrnoNoent,
		},
		{
			name:     "PathError ErrClosed",
			input:    &os.PathError{Err: fs.ErrClosed},
			expected: ErrnoBadf,
		},
		{
			name:     "PathError unknown == ErrnoIo",
			input:    &os.PathError{Err: errors.New("ice cream")},
			expected: ErrnoIo,
		},
		{
			name:     "unknown == ErrnoIo",
			input:    errors.New("ice cream"),
			expected: ErrnoIo,
		},
		{
			name:     "very wrapped unknown == ErrnoIo",
			input:    fmt.Errorf("%w", fmt.Errorf("%w", fmt.Errorf("%w", errors.New("ice cream")))),
			expected: ErrnoIo,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			errno := ToErrno(tc.input)
			require.Equal(t, tc.expected, errno)
		})
	}
}
