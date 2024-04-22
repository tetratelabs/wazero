package sys

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"testing"
	gofstest "testing/fstest"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata
var testdata embed.FS

func TestNewFSContext(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	dirfs := sysfs.DirFS(".")

	// Test various usual configuration for the file system.
	tests := []struct {
		name string
		fs   sys.FS
	}{
		{
			name: "embed.FS",
			fs:   &sysfs.AdaptFS{FS: embedFS},
		},
		{
			name: "DirFS",
			// Don't use "testdata" because it may not be present in
			// cross-architecture (a.k.a. scratch) build containers.
			fs: dirfs,
		},
		{
			name: "ReadFS",
			fs:   &sysfs.ReadFS{FS: dirfs},
		},
		{
			name: "fstest.MapFS",
			fs:   &sysfs.AdaptFS{FS: gofstest.MapFS{}},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			for _, root := range []string{"/", ""} {
				t.Run(fmt.Sprintf("root = '%s'", root), func(t *testing.T) {
					c := Context{}
					err := c.InitFSContext(nil, nil, nil, []sys.FS{tc.fs}, []string{root}, nil)
					require.NoError(t, err)
					fsc := c.fsc
					defer fsc.Close()

					rootFs := tc.fs
					preopenedDir, _ := fsc.openedFiles.Lookup(FdPreopen)
					require.Equal(t, tc.fs, rootFs)
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
		})

	}
}

func TestFSContext_CloseFile(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)
	testFS := &sysfs.AdaptFS{FS: embedFS}

	c := Context{}
	err = c.InitFSContext(nil, nil, nil, []sys.FS{testFS}, []string{"/"}, nil)
	require.NoError(t, err)
	fsc := c.fsc
	defer fsc.Close()

	fdToClose, errno := fsc.OpenFile(testFS, "empty.txt", sys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	fdToKeep, errno := fsc.OpenFile(testFS, "test.txt", sys.O_RDONLY, 0)
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
		require.EqualErrno(t, sys.EBADF, fsc.CloseFile(42)) // 42 is an arbitrary invalid FD
	})
	t.Run("Can close a pre-open", func(t *testing.T) {
		require.EqualErrno(t, 0, fsc.CloseFile(FdPreopen))
	})
}

func TestFSContext_noPreopens(t *testing.T) {
	c := Context{}
	err := c.InitFSContext(nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	testFS := &c.fsc
	require.NoError(t, err)

	expected := &FSContext{}
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
		require.Equal(t, &FSContext{}, testFS)
	})
}

func TestContext_Close(t *testing.T) {
	testFS := &sysfs.AdaptFS{FS: testfs.FS{"foo": &testfs.File{}}}

	c := Context{}
	err := c.InitFSContext(nil, nil, nil, []sys.FS{testFS}, []string{"/"}, nil)
	require.NoError(t, err)
	fsc := c.fsc

	// Verify base case
	require.Equal(t, 1+FdPreopen, int32(fsc.openedFiles.Len()))

	_, errno := fsc.OpenFile(testFS, "foo", sys.O_RDONLY, 0)
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

	testFS := &sysfs.AdaptFS{FS: testfs.FS{"foo": file}}

	c := Context{}
	err := c.InitFSContext(nil, nil, nil, []sys.FS{testFS}, []string{"/"}, nil)
	require.NoError(t, err)
	fsc := c.fsc

	// open another file
	_, errno := fsc.OpenFile(testFS, "foo", sys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// arbitrary errors coerce to EIO
	require.EqualErrno(t, sys.EIO, fsc.Close())

	// Paths should clear even under error
	require.Zero(t, fsc.openedFiles.Len(), "expected no opened files")
}

func TestFSContext_Renumber(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS := sysfs.DirFS(tmpDir)

	const dirName = "dir"
	errno := dirFS.Mkdir(dirName, 0o700)
	require.EqualErrno(t, 0, errno)

	c := Context{}
	err := c.InitFSContext(nil, nil, nil, []sys.FS{dirFS}, []string{"/"}, nil)
	require.NoError(t, err)
	fsc := c.fsc

	defer fsc.Close()

	for _, toFd := range []int32{10, 100, 100} {
		fromFd, errno := fsc.OpenFile(dirFS, dirName, sys.O_RDONLY, 0)
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
		require.Equal(t, sys.ENOTSUP, fsc.Renumber(3, 100))

		// From does not exist.
		require.Equal(t, sys.EBADF, fsc.Renumber(12345, 3))

		// Both are preopen.
		require.Equal(t, sys.ENOTSUP, fsc.Renumber(3, 3))
	})
}

// This is similar to https://github.com/WebAssembly/wasi-testsuite/blob/ac32f57400cdcdd0425d3085c24fc7fc40011d1c/tests/rust/src/bin/fd_readdir.rs#L120
func TestDirentCache_ReadNewFile(t *testing.T) {
	tmpDir := t.TempDir()

	c := Context{}
	root := sysfs.DirFS(tmpDir)
	err := c.InitFSContext(nil, nil, nil, []sys.FS{root}, []string{"/"}, nil)
	require.NoError(t, err)
	fsc := c.fsc
	defer fsc.Close()

	fd, errno := fsc.OpenFile(root, ".", sys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	defer fsc.CloseFile(fd) // nolint
	f, _ := fsc.LookupFile(fd)

	dir, errno := f.DirentCache()
	require.EqualErrno(t, 0, errno)

	// Read the empty directory, which should only have the dot entries.
	dirents, errno := dir.Read(0, 5)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, 2, len(dirents))
	require.Equal(t, ".", dirents[0].Name)
	require.Equal(t, "..", dirents[1].Name)

	// Write a new file to the directory
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "file"), nil, 0o0666))

	// Read it again, which should see the new file.
	dirents, errno = dir.Read(0, 5)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, 3, len(dirents))
	require.Equal(t, ".", dirents[0].Name)
	require.Equal(t, "..", dirents[1].Name)
	require.Equal(t, "file", dirents[2].Name)

	// Read it again, using the file position.
	filePos := uint64(2)
	dirents, errno = dir.Read(filePos, 3)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, 1, len(dirents))
	require.Equal(t, "file", dirents[0].Name)
}

func TestStripPrefixesAndTrailingSlash(t *testing.T) {
	tests := []struct {
		path, expected string
	}{
		{
			path:     ".",
			expected: "",
		},
		{
			path:     "/",
			expected: "",
		},
		{
			path:     "./",
			expected: "",
		},
		{
			path:     "./foo",
			expected: "foo",
		},
		{
			path:     ".foo",
			expected: ".foo",
		},
		{
			path:     "././foo",
			expected: "foo",
		},
		{
			path:     "/foo",
			expected: "foo",
		},
		{
			path:     "foo/",
			expected: "foo",
		},
		{
			path:     "//",
			expected: "",
		},
		{
			path:     "../../",
			expected: "../..",
		},
		{
			path:     "./../../",
			expected: "../..",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.path, func(t *testing.T) {
			path := StripPrefixesAndTrailingSlash(tc.path)
			require.Equal(t, tc.expected, path)
		})
	}
}
