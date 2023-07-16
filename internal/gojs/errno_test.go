package gojs

import (
	"io"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
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
			name:     "sys.EACCES",
			input:    sys.EACCES,
			expected: ErrnoAcces,
		},
		{
			name:     "sys.EAGAIN",
			input:    sys.EAGAIN,
			expected: ErrnoAgain,
		},
		{
			name:     "sys.EBADF",
			input:    sys.EBADF,
			expected: ErrnoBadf,
		},
		{
			name:     "sys.EEXIST",
			input:    sys.EEXIST,
			expected: ErrnoExist,
		},
		{
			name:     "sys.EFAULT",
			input:    sys.EFAULT,
			expected: ErrnoFault,
		},
		{
			name:     "sys.EINTR",
			input:    sys.EINTR,
			expected: ErrnoIntr,
		},
		{
			name:     "sys.EINVAL",
			input:    sys.EINVAL,
			expected: ErrnoInval,
		},
		{
			name:     "sys.EIO",
			input:    sys.EIO,
			expected: ErrnoIo,
		},
		{
			name:     "sys.EISDIR",
			input:    sys.EISDIR,
			expected: ErrnoIsdir,
		},
		{
			name:     "sys.ELOOP",
			input:    sys.ELOOP,
			expected: ErrnoLoop,
		},
		{
			name:     "sys.ENAMETOOLONG",
			input:    sys.ENAMETOOLONG,
			expected: ErrnoNametoolong,
		},
		{
			name:     "sys.ENOENT",
			input:    sys.ENOENT,
			expected: ErrnoNoent,
		},
		{
			name:     "sys.ENOSYS",
			input:    sys.ENOSYS,
			expected: ErrnoNosys,
		},
		{
			name:     "sys.ENOTDIR",
			input:    sys.ENOTDIR,
			expected: ErrnoNotdir,
		},
		{
			name:     "sys.ENOTEMPTY",
			input:    sys.ENOTEMPTY,
			expected: ErrnoNotempty,
		},
		{
			name:     "sys.ENOTSUP",
			input:    sys.ENOTSUP,
			expected: ErrnoNotsup,
		},
		{
			name:     "sys.EPERM",
			input:    sys.EPERM,
			expected: ErrnoPerm,
		},
		{
			name:     "sys.EROFS",
			input:    sys.EROFS,
			expected: ErrnoRofs,
		},
		{
			name:     "sys.Errno unexpected == ErrnoIo",
			input:    sys.Errno(0xfe),
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
