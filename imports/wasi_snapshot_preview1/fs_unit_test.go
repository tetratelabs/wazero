package wasi_snapshot_preview1

import (
	"os"
	"testing"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
)

func Test_maxDirents(t *testing.T) {
	tests := []struct {
		name                 string
		dirents              []experimentalsys.Dirent
		bufLen               uint32
		expectedBufToWrite   uint32
		expectedDirentCount  int
		expectedTruncatedLen uint32
	}{
		{
			name: "no entries",
		},
		{
			name:                 "can't fit one",
			dirents:              testDirents,
			bufLen:               23,
			expectedBufToWrite:   23,
			expectedDirentCount:  1,
			expectedTruncatedLen: 23,
		},
		{
			name:                 "only fits header",
			dirents:              testDirents,
			bufLen:               wasip1.DirentSize,
			expectedBufToWrite:   wasip1.DirentSize,
			expectedDirentCount:  1,
			expectedTruncatedLen: wasip1.DirentSize,
		},
		{
			name:                "one",
			dirents:             testDirents,
			bufLen:              25,
			expectedBufToWrite:  25,
			expectedDirentCount: 1,
		},
		{
			name:                 "one but not room for two's name",
			dirents:              testDirents,
			bufLen:               25 + 25,
			expectedBufToWrite:   25 + wasip1.DirentSize,
			expectedDirentCount:  2,
			expectedTruncatedLen: wasip1.DirentSize, // can write DirentSize
		},
		{
			name:                "two",
			dirents:             testDirents,
			bufLen:              25 + 26,
			expectedBufToWrite:  25 + 26,
			expectedDirentCount: 2,
		},
		{
			name:                 "two but not three's dirent",
			dirents:              testDirents,
			bufLen:               25 + 26 + 20,
			expectedBufToWrite:   25 + 26 + 20,
			expectedDirentCount:  3,
			expectedTruncatedLen: 20, // 20 + 4 == DirentSize
		},
		{
			name:                 "two but not three's name",
			dirents:              testDirents,
			bufLen:               25 + 26 + 25,
			expectedBufToWrite:   25 + 26 + wasip1.DirentSize,
			expectedDirentCount:  3,
			expectedTruncatedLen: wasip1.DirentSize, // can write DirentSize
		},
		{
			name:                "three",
			dirents:             testDirents,
			bufLen:              25 + 26 + 27,
			expectedBufToWrite:  25 + 26 + 27,
			expectedDirentCount: 3,
		},
		{
			name:                "max",
			dirents:             testDirents,
			bufLen:              100,
			expectedBufToWrite:  25 + 26 + 27,
			expectedDirentCount: 3,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bufToWrite, direntCount, truncatedLen := maxDirents(tc.dirents, tc.bufLen)
			require.Equal(t, tc.expectedBufToWrite, bufToWrite)
			require.Equal(t, tc.expectedDirentCount, direntCount)
			require.Equal(t, tc.expectedTruncatedLen, truncatedLen)
		})
	}
}

var (
	testDirents = func() []experimentalsys.Dirent {
		dPath := "dir"
		d, errno := sysfs.OpenFSFile(fstest.FS, dPath, experimentalsys.O_RDONLY, 0)
		if errno != 0 {
			panic(errno)
		}
		defer d.Close()
		dirents, errno := d.Readdir(-1)
		if errno != 0 {
			panic(errno)
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
		name         string
		dirents      []experimentalsys.Dirent
		entryCount   int
		truncatedLen uint32
		expected     []byte
	}{
		{
			name:    "none",
			dirents: testDirents,
		},
		{
			name:       "one",
			dirents:    testDirents,
			entryCount: 1,
			expected:   dirent1,
		},
		{
			name:       "two",
			dirents:    testDirents,
			entryCount: 2,
			expected:   append(dirent1, dirent2...),
		},
		{
			name:         "two with truncated dirent",
			dirents:      testDirents,
			entryCount:   3,
			truncatedLen: wasip1.DirentSize,
			expected:     append(append(dirent1, dirent2...), dirent3[:wasip1.DirentSize]...),
		},
		{
			name:         "two with truncated smaller than dirent",
			dirents:      testDirents,
			entryCount:   3,
			truncatedLen: 5,
			expected:     append(append(dirent1, dirent2...), 0, 0, 0, 0, 0),
		},
		{
			name:       "three",
			dirents:    testDirents,
			entryCount: 3,
			expected:   append(append(dirent1, dirent2...), dirent3...),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			d_next := uint64(1)
			buf := make([]byte, len(tc.expected))
			writeDirents(buf, tc.dirents, d_next, tc.entryCount, tc.truncatedLen)
			require.Equal(t, tc.expected, buf)
		})
	}
}

func Test_openFlags(t *testing.T) {
	tests := []struct {
		name                      string
		dirflags, oflags, fdflags uint16
		rights                    uint32
		expectedOpenFlags         experimentalsys.Oflag
	}{
		{
			name:              "oflags=0",
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDONLY,
		},
		{
			name:              "oflags=O_CREAT",
			oflags:            wasip1.O_CREAT,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDWR | experimentalsys.O_CREAT,
		},
		{
			name:              "oflags=O_DIRECTORY",
			oflags:            wasip1.O_DIRECTORY,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_DIRECTORY,
		},
		{
			name:              "oflags=O_EXCL",
			oflags:            wasip1.O_EXCL,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDONLY | experimentalsys.O_EXCL,
		},
		{
			name:              "oflags=O_TRUNC",
			oflags:            wasip1.O_TRUNC,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDWR | experimentalsys.O_TRUNC,
		},
		{
			name:              "fdflags=FD_APPEND",
			fdflags:           wasip1.FD_APPEND,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDWR | experimentalsys.O_APPEND,
		},
		{
			name:              "oflags=O_TRUNC|O_CREAT",
			oflags:            wasip1.O_TRUNC | wasip1.O_CREAT,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDWR | experimentalsys.O_TRUNC | experimentalsys.O_CREAT,
		},
		{
			name:              "dirflags=LOOKUP_SYMLINK_FOLLOW",
			dirflags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			expectedOpenFlags: experimentalsys.O_RDONLY,
		},
		{
			name:              "rights=FD_READ",
			rights:            wasip1.RIGHT_FD_READ,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDONLY,
		},
		{
			name:              "rights=FD_WRITE",
			rights:            wasip1.RIGHT_FD_WRITE,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_WRONLY,
		},
		{
			name:              "rights=FD_READ|FD_WRITE",
			rights:            wasip1.RIGHT_FD_READ | wasip1.RIGHT_FD_WRITE,
			expectedOpenFlags: experimentalsys.O_NOFOLLOW | experimentalsys.O_RDWR,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			openFlags := openFlags(tc.dirflags, tc.oflags, tc.fdflags, tc.rights)
			require.Equal(t, tc.expectedOpenFlags, openFlags)
		})
	}
}

func Test_getWasiFiletype_DevNull(t *testing.T) {
	st, err := os.Stat(os.DevNull)
	require.NoError(t, err)

	ft := getWasiFiletype(st.Mode())

	// Should be a character device, and not contain permissions
	require.Equal(t, wasip1.FILETYPE_CHARACTER_DEVICE, ft)
}

func Test_isPreopenedStdio(t *testing.T) {
	tests := []struct {
		name     string
		fd       int32
		f        *sys.FileEntry
		expected bool
	}{
		{
			name:     "stdin",
			fd:       sys.FdStdin,
			f:        &sys.FileEntry{IsPreopen: true},
			expected: true,
		},
		{
			name:     "stdin re-opened",
			fd:       sys.FdStdin,
			f:        &sys.FileEntry{IsPreopen: false},
			expected: false,
		},
		{
			name:     "stdout",
			fd:       sys.FdStdout,
			f:        &sys.FileEntry{IsPreopen: true},
			expected: true,
		},
		{
			name:     "stdout re-opened",
			fd:       sys.FdStdout,
			f:        &sys.FileEntry{IsPreopen: false},
			expected: false,
		},
		{
			name:     "stderr",
			fd:       sys.FdStderr,
			f:        &sys.FileEntry{IsPreopen: true},
			expected: true,
		},
		{
			name:     "stderr re-opened",
			fd:       sys.FdStderr,
			f:        &sys.FileEntry{IsPreopen: false},
			expected: false,
		},
		{
			name:     "not stdio pre-open",
			fd:       sys.FdPreopen,
			f:        &sys.FileEntry{IsPreopen: true},
			expected: false,
		},
		{
			name:     "random file",
			fd:       42,
			f:        &sys.FileEntry{},
			expected: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ok := isPreopenedStdio(tc.fd, tc.f)
			require.Equal(t, tc.expected, ok)
		})
	}
}
