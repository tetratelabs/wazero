package require

import (
	"io"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
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
				EqualErrno(t, sys.ENOENT, sys.ENOENT)
			},
		},
		{
			name: "EqualErrno fails on nil",
			require: func(t TestingT) {
				EqualErrno(t, sys.ENOENT, nil)
			},
			expectedLog: "expected a sys.Errno, but was nil",
		},
		{
			name: "EqualErrno fails on not Errno",
			require: func(t TestingT) {
				EqualErrno(t, sys.ENOENT, io.EOF)
			},
			expectedLog: `expected EOF to be a sys.Errno`,
		},
		{
			name: "EqualErrno fails on not equal",
			require: func(t TestingT) {
				EqualErrno(t, sys.ENOENT, sys.EIO)
			},
			expectedLog: `expected Errno 0xc(no such file or directory), but was 0x8(input/output error)`,
		},
		{
			name: "EqualErrno fails on not equal with format",
			require: func(t TestingT) {
				EqualErrno(t, sys.ENOENT, sys.EIO, "pay me %d", 5)
			},
			expectedLog: `expected Errno 0xc(no such file or directory), but was 0x8(input/output error): pay me 5`,
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
