package require

import (
	"io"
	"runtime"
	"syscall"
	"testing"
)

func TestEqualErrno(t *testing.T) {
	// This specifically chooses ENOENT and EIO as outside windows, they tend
	// to have the same errno literal and text message.
	if runtime.GOOS == "windows" {
		t.Skipf("error literals are different on windows")
	}

	tests := []struct {
		name        string
		require     func(TestingT)
		expectedLog string
	}{
		{
			name: "EqualErrno passes on equal",
			require: func(t TestingT) {
				EqualErrno(t, syscall.ENOENT, syscall.ENOENT)
			},
		},
		{
			name: "EqualErrno fails on nil",
			require: func(t TestingT) {
				EqualErrno(t, syscall.ENOENT, nil)
			},
			expectedLog: "expected a syscall.Errno, but was nil",
		},
		{
			name: "EqualErrno fails on not Errno",
			require: func(t TestingT) {
				EqualErrno(t, syscall.ENOENT, io.EOF)
			},
			expectedLog: `expected EOF to be a syscall.Errno`,
		},
		{
			name: "EqualErrno fails on not equal",
			require: func(t TestingT) {
				EqualErrno(t, syscall.ENOENT, syscall.EIO)
			},
			expectedLog: `expected Errno 0x2(no such file or directory), but was 0x5(input/output error)`,
		},
		{
			name: "EqualErrno fails on not equal with format",
			require: func(t TestingT) {
				EqualErrno(t, syscall.ENOENT, syscall.EIO, "pay me %d", 5)
			},
			expectedLog: `expected Errno 0x2(no such file or directory), but was 0x5(input/output error): pay me 5`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &mockT{t: t}
			tc.require(m)
			m.require(tc.expectedLog)
		})
	}
}
