package wasi_snapshot_preview1

import (
	"io"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

func Test_fdRead_shouldContinueRead(t *testing.T) {
	tests := []struct {
		name          string
		n, l          uint32
		err           error
		expectedOk    bool
		expectedErrno Errno
	}{
		{
			name: "break when nothing to read",
			n:    0,
			l:    0,
		},
		{
			name: "break when nothing read",
			n:    0,
			l:    4,
		},
		{
			name: "break on partial read",
			n:    3,
			l:    4,
		},
		{
			name:       "continue on full read",
			n:          4,
			l:          4,
			expectedOk: true,
		},
		{
			name: "break on EOF on nothing to read",
			err:  io.EOF,
		},
		{
			name: "break on EOF on nothing read",
			l:    4,
			err:  io.EOF,
		},
		{
			name: "break on EOF on partial read",
			n:    3,
			l:    4,
			err:  io.EOF,
		},
		{
			name: "break on EOF on full read",
			n:    4,
			l:    4,
			err:  io.EOF,
		},
		{
			name:          "return ErrnoIo on error on nothing to read",
			err:           io.ErrClosedPipe,
			expectedErrno: ErrnoIo,
		},
		{
			name:          "return ErrnoIo on error on nothing read",
			l:             4,
			err:           io.ErrClosedPipe,
			expectedErrno: ErrnoIo,
		},
		{ // Special case, allows processing data before err
			name: "break on error on partial read",
			n:    3,
			l:    4,
			err:  io.ErrClosedPipe,
		},
		{ // Special case, allows processing data before err
			name: "break on error on full read",
			n:    4,
			l:    4,
			err:  io.ErrClosedPipe,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ok, errno := fdRead_shouldContinueRead(tc.n, tc.l, tc.err)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

func Test_lastDirents(t *testing.T) {
	tests := []struct {
		name            string
		f               *sys.ReadDir
		cookie          int64
		expectedDirents []*platform.Dirent
		expectedErrno   Errno
	}{
		{
			name: "no prior call",
		},
		{
			name:          "no prior call, but passed a cookie",
			cookie:        1,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is negative",
			f: &sys.ReadDir{
				CountRead: 3,
				Dirents:   testDirents,
			},
			cookie:        -1,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is greater than last d_next",
			f: &sys.ReadDir{
				CountRead: 3,
				Dirents:   testDirents,
			},
			cookie:        5,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is last pos",
			f: &sys.ReadDir{
				CountRead: 3,
				Dirents:   testDirents,
			},
			cookie:          3,
			expectedDirents: nil,
		},
		{
			name: "cookie is one before last pos",
			f: &sys.ReadDir{
				CountRead: 3,
				Dirents:   testDirents,
			},
			cookie:          2,
			expectedDirents: testDirents[2:],
		},
		{
			name: "cookie is before current entries",
			f: &sys.ReadDir{
				CountRead: 5,
				Dirents:   testDirents,
			},
			cookie:        1,
			expectedErrno: ErrnoNosys, // not implemented
		},
		{
			name: "read from the beginning (cookie=0)",
			f: &sys.ReadDir{
				CountRead: 3,
				Dirents:   testDirents,
			},
			cookie:          0,
			expectedDirents: testDirents,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if f == nil {
				f = &sys.ReadDir{}
			}
			entries, errno := lastDirents(f, tc.cookie)
			require.Equal(t, tc.expectedErrno, errno)
			require.Equal(t, tc.expectedDirents, entries)
		})
	}
}

func Test_maxDirents(t *testing.T) {
	tests := []struct {
		name                        string
		dirents                     []*platform.Dirent
		maxLen                      uint32
		expectedCount               uint32
		expectedwriteTruncatedEntry bool
		expectedBufused             uint32
	}{
		{
			name: "no entries",
		},
		{
			name:                        "can't fit one",
			dirents:                     testDirents,
			maxLen:                      23,
			expectedBufused:             23,
			expectedwriteTruncatedEntry: false,
		},
		{
			name:                        "only fits header",
			dirents:                     testDirents,
			maxLen:                      24,
			expectedBufused:             24,
			expectedwriteTruncatedEntry: true,
		},
		{
			name:            "one",
			dirents:         testDirents,
			maxLen:          25,
			expectedCount:   1,
			expectedBufused: 25,
		},
		{
			name:                        "one but not room for two's name",
			dirents:                     testDirents,
			maxLen:                      25 + 25,
			expectedCount:               1,
			expectedwriteTruncatedEntry: true, // can write DirentSize
			expectedBufused:             25 + 25,
		},
		{
			name:            "two",
			dirents:         testDirents,
			maxLen:          25 + 26,
			expectedCount:   2,
			expectedBufused: 25 + 26,
		},
		{
			name:                        "two but not three's dirent",
			dirents:                     testDirents,
			maxLen:                      25 + 26 + 20,
			expectedCount:               2,
			expectedwriteTruncatedEntry: false, // 20 + 4 == DirentSize
			expectedBufused:             25 + 26 + 20,
		},
		{
			name:                        "two but not three's name",
			dirents:                     testDirents,
			maxLen:                      25 + 26 + 26,
			expectedCount:               2,
			expectedwriteTruncatedEntry: true, // can write DirentSize
			expectedBufused:             25 + 26 + 26,
		},
		{
			name:                        "three",
			dirents:                     testDirents,
			maxLen:                      25 + 26 + 27,
			expectedCount:               3,
			expectedwriteTruncatedEntry: false, // end of dir
			expectedBufused:             25 + 26 + 27,
		},
		{
			name:                        "max",
			dirents:                     testDirents,
			maxLen:                      100,
			expectedCount:               3,
			expectedwriteTruncatedEntry: false, // end of dir
			expectedBufused:             25 + 26 + 27,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bufused, direntCount, writeTruncatedEntry := maxDirents(tc.dirents, tc.maxLen)
			require.Equal(t, tc.expectedCount, direntCount)
			require.Equal(t, tc.expectedwriteTruncatedEntry, writeTruncatedEntry)
			require.Equal(t, tc.expectedBufused, bufused)
		})
	}
}

var (
	testDirents = func() []*platform.Dirent {
		dir, err := fstest.FS.Open("dir")
		if err != nil {
			panic(err)
		}
		defer dir.Close()
		dirents, err := platform.Readdir(dir, -1)
		if err != nil {
			panic(err)
		}
		return dirents
	}()

	dirent1 = []byte{
		1, 0, 0, 0, 0, 0, 0, 0, // d_next = 1
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		1, 0, 0, 0, // d_namlen = 1 character
		4, 0, 0, 0, // d_type = regular_file
		'-', // name
	}
	dirent2 = []byte{
		2, 0, 0, 0, 0, 0, 0, 0, // d_next = 2
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		2, 0, 0, 0, // d_namlen = 1 character
		3, 0, 0, 0, // d_type =  directory
		'a', '-', // name
	}
	dirent3 = []byte{
		3, 0, 0, 0, 0, 0, 0, 0, // d_next = 3
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		3, 0, 0, 0, // d_namlen = 3 characters
		4, 0, 0, 0, // d_type = regular_file
		'a', 'b', '-', // name
	}
)

func Test_writeDirents(t *testing.T) {
	tests := []struct {
		name                string
		entries             []*platform.Dirent
		entryCount          uint32
		writeTruncatedEntry bool
		expectedEntriesBuf  []byte
	}{
		{
			name:    "none",
			entries: testDirents,
		},
		{
			name:               "one",
			entries:            testDirents,
			entryCount:         1,
			expectedEntriesBuf: dirent1,
		},
		{
			name:               "two",
			entries:            testDirents,
			entryCount:         2,
			expectedEntriesBuf: append(dirent1, dirent2...),
		},
		{
			name:                "two with truncated",
			entries:             testDirents,
			entryCount:          2,
			writeTruncatedEntry: true,
			expectedEntriesBuf:  append(append(dirent1, dirent2...), dirent3[0:10]...),
		},
		{
			name:               "three",
			entries:            testDirents,
			entryCount:         3,
			expectedEntriesBuf: append(append(dirent1, dirent2...), dirent3...),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			cookie := uint64(1)
			entriesBuf := make([]byte, len(tc.expectedEntriesBuf))
			writeDirents(tc.entries, tc.entryCount, tc.writeTruncatedEntry, entriesBuf, cookie)
			require.Equal(t, tc.expectedEntriesBuf, entriesBuf)
		})
	}
}

func Test_openFlags(t *testing.T) {
	tests := []struct {
		name                      string
		dirflags, oflags, fdflags uint16
		expectedOpenFlags         int
	}{
		{
			name:              "oflags=0",
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDONLY,
		},
		{
			name:              "oflags=O_CREAT",
			oflags:            O_CREAT,
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDWR | syscall.O_CREAT,
		},
		{
			name:              "oflags=O_DIRECTORY",
			oflags:            O_DIRECTORY,
			expectedOpenFlags: platform.O_NOFOLLOW | platform.O_DIRECTORY,
		},
		{
			name:              "oflags=O_EXCL",
			oflags:            O_EXCL,
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDONLY | syscall.O_EXCL,
		},
		{
			name:              "oflags=O_TRUNC",
			oflags:            O_TRUNC,
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDWR | syscall.O_TRUNC,
		},
		{
			name:              "fdflags=FD_APPEND",
			fdflags:           FD_APPEND,
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDWR | syscall.O_APPEND,
		},
		{
			name:              "oflags=O_TRUNC|O_CREAT",
			oflags:            O_TRUNC | O_CREAT,
			expectedOpenFlags: platform.O_NOFOLLOW | syscall.O_RDWR | syscall.O_TRUNC | syscall.O_CREAT,
		},
		{
			name:              "dirflags=LOOKUP_SYMLINK_FOLLOW",
			dirflags:          LOOKUP_SYMLINK_FOLLOW,
			expectedOpenFlags: syscall.O_RDONLY,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			openFlags := openFlags(tc.dirflags, tc.oflags, tc.fdflags)
			require.Equal(t, tc.expectedOpenFlags, openFlags)
		})
	}
}
