package gojs

import (
	"io"
	"syscall"
	"testing"
)

func TestToErrno(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected *Errno
	}{
		{
			name: "nil is not an error",
		},
		{
			name:  "io.EOF is not an error",
			input: io.EOF,
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
			name:     "syscall.Errno unexpected == ErrnoIo",
			input:    syscall.Errno(0xfe),
			expected: ErrnoIo,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if errno := ToErrno(tc.input); errno != tc.expected {
				t.Fatalf("expected %#v but was %#v", tc.expected, errno)
			}
		})
	}
}
