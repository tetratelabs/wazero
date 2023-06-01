package sys

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata
var testdata embed.FS

func TestNewFSContext(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	dirfs := sysfs.NewDirFS(".")

	// Test various usual configuration for the file system.
	tests := []struct {
		name string
		fs   fsapi.FS
	}{
		{
			name: "embed.FS",
			fs:   sysfs.Adapt(embedFS),
		},
		{
			name: "NewDirFS",
			// Don't use "testdata" because it may not be present in
			// cross-architecture (a.k.a. scratch) build containers.
			fs: dirfs,
		},
		{
			name: "NewReadFS",
			fs:   sysfs.NewReadFS(dirfs),
		},
		{
			name: "fstest.MapFS",
			fs:   sysfs.Adapt(fstest.MapFS{}),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			c := Context{}
			err := c.NewFSContext(nil, nil, nil, tc.fs, nil)
			require.NoError(t, err)
			fsc := c.fsc
			defer fsc.Close()

			preopenedDir, _ := fsc.openedFiles.Lookup(FdPreopen)
			require.Equal(t, tc.fs, fsc.rootFS)
			require.NotNil(t, preopenedDir)
			require.Equal(t, "/", preopenedDir.Name)

			// Verify that each call to OpenFile returns a different file
			// descriptor.
			f1, errno := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.EqualErrno(t, 0, errno)
			require.NotEqual(t, FdPreopen, f1)

			// Verify that file descriptors are reused.
			//
			// Note that this specific behavior is not required by WASI which
			// only documents that file descriptor numbers will be selected
			// randomly and applications should not rely on them. We added this
			// test to ensure that our implementation properly reuses descriptor
			// numbers but if we were to change the reuse strategy, this test
			// would likely break and need to be updated.
			require.EqualErrno(t, 0, fsc.CloseFile(f1))
			f2, errno := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, f1, f2)
		})
	}
}

func TestFSContext_CloseFile(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)
	testFS := sysfs.Adapt(embedFS)

	c := Context{}
	err = c.NewFSContext(nil, nil, nil, testFS, nil)
	require.NoError(t, err)
	fsc := c.fsc
	defer fsc.Close()

	fdToClose, errno := fsc.OpenFile(testFS, "empty.txt", os.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	fdToKeep, errno := fsc.OpenFile(testFS, "test.txt", os.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// Close
	require.EqualErrno(t, 0, fsc.CloseFile(fdToClose))

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.LookupFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.LookupFile(fdToKeep)
	require.True(t, ok)

	t.Run("EBADF for an invalid FD", func(t *testing.T) {
		require.EqualErrno(t, syscall.EBADF, fsc.CloseFile(42)) // 42 is an arbitrary invalid FD
	})
	t.Run("Can close a pre-open", func(t *testing.T) {
		require.EqualErrno(t, 0, fsc.CloseFile(FdPreopen))
	})
}

func TestUnimplementedFSContext(t *testing.T) {
	c := Context{}
	err := c.NewFSContext(nil, nil, nil, fsapi.UnimplementedFS{}, nil)
	require.NoError(t, err)
	testFS := &c.fsc
	require.NoError(t, err)

	expected := &FSContext{rootFS: fsapi.UnimplementedFS{}}
	noopStdin, _ := stdinFileEntry(nil)
	expected.openedFiles.Insert(noopStdin)
	noopStdout, _ := stdioWriterFileEntry("stdout", nil)
	expected.openedFiles.Insert(noopStdout)
	noopStderr, _ := stdioWriterFileEntry("stderr", nil)
	expected.openedFiles.Insert(noopStderr)

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close()
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{rootFS: fsapi.UnimplementedFS{}}, testFS)
	})
}

func TestCompositeFSContext(t *testing.T) {
	tmpDir1 := t.TempDir()
	testFS1 := sysfs.NewDirFS(tmpDir1)

	tmpDir2 := t.TempDir()
	testFS2 := sysfs.NewDirFS(tmpDir2)

	rootFS, err := sysfs.NewRootFS([]fsapi.FS{testFS2, testFS1}, []string{"/tmp", "/"})
	require.NoError(t, err)

	c := Context{}
	err = c.NewFSContext(nil, nil, nil, rootFS, nil)
	require.NoError(t, err)
	testFS := &c.fsc

	// Ensure the pre-opens have exactly the name specified, and are in order.
	preopen3, ok := testFS.openedFiles.Lookup(3)
	require.True(t, ok)
	require.Equal(t, "/tmp", preopen3.Name)
	preopen4, ok := testFS.openedFiles.Lookup(4)
	require.True(t, ok)
	require.Equal(t, "/", preopen4.Name)

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close()
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{rootFS: rootFS}, testFS)
	})
}

func TestContext_Close(t *testing.T) {
	testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{}})

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, testFS, nil)
	require.NoError(t, err)
	fsc := c.fsc

	// Verify base case
	require.Equal(t, 1+FdPreopen, int32(fsc.openedFiles.Len()))

	_, errno := fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, 2+FdPreopen, int32(fsc.openedFiles.Len()))

	// Closing should not err.
	require.NoError(t, fsc.Close())

	// Verify our intended side-effect
	require.Zero(t, fsc.openedFiles.Len())

	// Verify no error closing again.
	require.NoError(t, fsc.Close())
}

func TestContext_Close_Error(t *testing.T) {
	file := &testfs.File{CloseErr: errors.New("error closing")}

	testFS := sysfs.Adapt(testfs.FS{"foo": file})

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, testFS, nil)
	require.NoError(t, err)
	fsc := c.fsc

	// open another file
	_, errno := fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// arbitrary errors coerce to EIO
	require.EqualErrno(t, syscall.EIO, fsc.Close())

	// Paths should clear even under error
	require.Zero(t, fsc.openedFiles.Len(), "expected no opened files")
}

func TestFSContext_Renumber(t *testing.T) {
	tmpDir := t.TempDir()
	dirFs := sysfs.NewDirFS(tmpDir)

	const dirName = "dir"
	errno := dirFs.Mkdir(dirName, 0o700)
	require.EqualErrno(t, 0, errno)

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, dirFs, nil)
	require.NoError(t, err)
	fsc := c.fsc

	defer fsc.Close()

	for _, toFd := range []int32{10, 100, 100} {
		fromFd, errno := fsc.OpenFile(dirFs, dirName, os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)

		prevDirFile, ok := fsc.LookupFile(fromFd)
		require.True(t, ok)

		require.EqualErrno(t, 0, fsc.Renumber(fromFd, toFd))

		renumberedDirFile, ok := fsc.LookupFile(toFd)
		require.True(t, ok)

		require.Equal(t, prevDirFile, renumberedDirFile)

		// Previous file descriptor shouldn't be used.
		_, ok = fsc.LookupFile(fromFd)
		require.False(t, ok)
	}

	t.Run("errors", func(t *testing.T) {
		// Sanity check for 3 being preopen.
		preopen, ok := fsc.LookupFile(3)
		require.True(t, ok)
		require.True(t, preopen.IsPreopen)

		// From is preopen.
		require.Equal(t, syscall.ENOTSUP, fsc.Renumber(3, 100))

		// From does not exist.
		require.Equal(t, syscall.EBADF, fsc.Renumber(12345, 3))

		// Both are preopen.
		require.Equal(t, syscall.ENOTSUP, fsc.Renumber(3, 3))
	})
}

func TestReaddDir_Rewind(t *testing.T) {
	tests := []struct {
		name           string
		f              *Readdir
		cookie         int64
		expectedCookie int64
		expectedErrno  syscall.Errno
	}{
		{
			name: "no prior call",
		},
		{
			name:          "no prior call, but passed a cookie",
			cookie:        1,
			expectedErrno: syscall.EINVAL,
		},
		{
			name: "cookie is negative",
			f: &Readdir{
				countRead: 3,
			},
			cookie:        -1,
			expectedErrno: syscall.EINVAL,
		},
		{
			name: "cookie is greater than last d_next",
			f: &Readdir{
				countRead: 3,
			},
			cookie:        5,
			expectedErrno: syscall.EINVAL,
		},
		{
			name: "cookie is last pos",
			f: &Readdir{
				countRead: 3,
			},
			cookie: 3,
		},
		{
			name: "cookie is one before last pos",
			f: &Readdir{
				countRead: 3,
			},
			cookie: 2,
		},
		{
			name: "cookie is before current entries",
			f: &Readdir{
				countRead: direntBufSize + 2,
			},
			cookie:        1,
			expectedErrno: syscall.ENOSYS, // not implemented
		},
		{
			name: "read from the beginning (cookie=0)",
			f: &Readdir{
				dirInit: func() ([]fsapi.Dirent, syscall.Errno) {
					return []fsapi.Dirent{{Name: "."}, {Name: ".."}}, 0
				},
				dirReader: func(n uint64) ([]fsapi.Dirent, syscall.Errno) {
					return nil, 0
				},
				cursor: 3,
			},
			cookie: 0,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if f == nil {
				f = &Readdir{}
			}

			errno := f.Rewind(tc.cookie)
			require.EqualErrno(t, tc.expectedErrno, errno)
		})
	}
}
