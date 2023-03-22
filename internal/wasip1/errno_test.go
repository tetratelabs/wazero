package wasip1

import (
	"syscall"
	"testing"
)

func TestToErrno(t *testing.T) {
	tests := []struct {
		name     string
		input    syscall.Errno
		expected Errno
	}{
		{
			name:     "zero is not an error",
			expected: ErrnoSuccess,
		},
		{
			name:     "syscall.EACCES",
			input:    syscall.EACCES,
			expected: ErrnoAcces,
		},
		{
			name:     "syscall.EAGAIN",
			input:    syscall.EAGAIN,
			expected: ErrnoAgain,
		},
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
			name:     "syscall.EFAULT",
			input:    syscall.EFAULT,
			expected: ErrnoFault,
		},
		{
			name:     "syscall.EINTR",
			input:    syscall.EINTR,
			expected: ErrnoIntr,
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
			name:     "syscall.EROFS",
			input:    syscall.EROFS,
			expected: ErrnoRofs,
		},
		{
			name:     "syscall.EqualErrno unexpected == ErrnoIo",
			input:    syscall.Errno(0xfe),
			expected: ErrnoIo,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if errno := ToErrno(tc.input); errno != tc.expected {
				t.Fatalf("expected %s but got %s", ErrnoName(tc.expected), ErrnoName(errno))
			}
		})
	}
}
