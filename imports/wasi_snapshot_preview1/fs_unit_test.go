package wasi_snapshot_preview1

import (
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
)

func Test_maxDirents(t *testing.T) {
	tests := []struct {
		name                        string
		dirents                     []fsapi.Dirent
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
			readdir := sysfs.NewReaddir(tc.dirents...)
			_, bufused, direntCount, writeTruncatedEntry := maxDirents(readdir, tc.maxLen)
			require.Equal(t, tc.expectedCount, direntCount)
			require.Equal(t, tc.expectedwriteTruncatedEntry, writeTruncatedEntry)
			require.Equal(t, tc.expectedBufused, bufused)
		})
	}
}

var (
	testDirents = func() []fsapi.Dirent {
		dPath := "dir"
		d, errno := sysfs.OpenFSFile(fstest.FS, dPath, syscall.O_RDONLY, 0)
		if errno != 0 {
			panic(errno)
		}
		defer d.Close()
		dirs, errno := d.Readdir()
		if errno != 0 {
			panic(errno)
		}
		dirents, errno := sysfs.ReaddirAll(dirs)
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
		name                string
		entries             []fsapi.Dirent
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
		rights                    uint32
		expectedOpenFlags         int
	}{
		{
			name:              "oflags=0",
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDONLY,
		},
		{
			name:              "oflags=O_CREAT",
			oflags:            wasip1.O_CREAT,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDWR | syscall.O_CREAT,
		},
		{
			name:              "oflags=O_DIRECTORY",
			oflags:            wasip1.O_DIRECTORY,
			expectedOpenFlags: fsapi.O_NOFOLLOW | fsapi.O_DIRECTORY,
		},
		{
			name:              "oflags=O_EXCL",
			oflags:            wasip1.O_EXCL,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDONLY | syscall.O_EXCL,
		},
		{
			name:              "oflags=O_TRUNC",
			oflags:            wasip1.O_TRUNC,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDWR | syscall.O_TRUNC,
		},
		{
			name:              "fdflags=FD_APPEND",
			fdflags:           wasip1.FD_APPEND,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDWR | syscall.O_APPEND,
		},
		{
			name:              "oflags=O_TRUNC|O_CREAT",
			oflags:            wasip1.O_TRUNC | wasip1.O_CREAT,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDWR | syscall.O_TRUNC | syscall.O_CREAT,
		},
		{
			name:              "dirflags=LOOKUP_SYMLINK_FOLLOW",
			dirflags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			expectedOpenFlags: syscall.O_RDONLY,
		},
		{
			name:              "rights=FD_READ",
			rights:            wasip1.RIGHT_FD_READ,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDONLY,
		},
		{
			name:              "rights=FD_WRITE",
			rights:            wasip1.RIGHT_FD_WRITE,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_WRONLY,
		},
		{
			name:              "rights=FD_READ|FD_WRITE",
			rights:            wasip1.RIGHT_FD_READ | wasip1.RIGHT_FD_WRITE,
			expectedOpenFlags: fsapi.O_NOFOLLOW | syscall.O_RDWR,
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
