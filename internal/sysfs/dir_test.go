package sysfs_test

import (
	"io"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFSFileReaddir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	dirFS := os.DirFS(tmpDir)
	maskFS := &sysfs.MaskOsFS{Fs: dirFS}
	maskFSZeroIno := &sysfs.MaskOsFS{Fs: os.DirFS(tmpDir), ZeroIno: true}

	expectIno := runtime.GOOS != "windows"

	tests := []struct {
		name      string
		fs        fs.FS
		expectIno bool
	}{
		{name: "os.DirFS", fs: dirFS, expectIno: expectIno},                   // To test readdirFile
		{name: "mask(os.DirFS)", fs: maskFS, expectIno: expectIno},            // To prove no reliance on os.File
		{name: "mask(os.DirFS) ZeroIno", fs: maskFSZeroIno, expectIno: false}, // To prove Stat_t overrides
		{name: "fstest.MapFS", fs: fstest.FS, expectIno: false},               // To test adaptation of ReadDirFile
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			dotF, errno := sysfs.OpenFSFile(tc.fs, ".", sys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer dotF.Close()

			t.Run("dir", func(t *testing.T) {
				testReaddirAll(t, dotF, tc.expectIno)

				// read again even though it is exhausted
				dirents, errno := dotF.Readdir(100)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, len(dirents))

				// rewind via seek to zero
				newOffset, errno := dotF.Seek(0, io.SeekStart)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, newOffset)

				// redundantly seek to zero again
				newOffset, errno = dotF.Seek(0, io.SeekStart)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, newOffset)

				// We should be able to read again
				testReaddirAll(t, dotF, tc.expectIno)
			})

			// Err if the caller closed the directory while reading. This is
			// different from something else deleting it.
			t.Run("closed dir", func(t *testing.T) {
				require.EqualErrno(t, 0, dotF.Close())
				_, errno := dotF.Readdir(-1)
				require.EqualErrno(t, sys.EBADF, errno)
			})

			fileF, errno := sysfs.OpenFSFile(tc.fs, "empty.txt", sys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, errno := fileF.Readdir(-1)
				require.EqualErrno(t, sys.EBADF, errno)
			})

			dirF, errno := sysfs.OpenFSFile(tc.fs, "dir", sys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer dirF.Close()

			t.Run("partial", func(t *testing.T) {
				dirents1, errno := dirF.Readdir(1)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, 1, len(dirents1))

				dirents2, errno := dirF.Readdir(1)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, 1, len(dirents2))

				// read exactly the last entry
				dirents3, errno := dirF.Readdir(1)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, 1, len(dirents3))

				dirents := []sys.Dirent{dirents1[0], dirents2[0], dirents3[0]}
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				requireIno(t, dirents, tc.expectIno)

				// Scrub inodes so we can compare expectations without them.
				for i := range dirents {
					dirents[i].Ino = 0
				}

				require.Equal(t, []sys.Dirent{
					{Name: "-", Type: 0},
					{Name: "a-", Type: fs.ModeDir},
					{Name: "ab-", Type: 0},
				}, dirents)

				// no error reading an exhausted directory
				_, errno = dirF.Readdir(1)
				require.EqualErrno(t, 0, errno)
			})

			subdirF, errno := sysfs.OpenFSFile(tc.fs, "sub", sys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer subdirF.Close()

			t.Run("subdir", func(t *testing.T) {
				dirents, errno := subdirF.Readdir(-1)
				require.EqualErrno(t, 0, errno)
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				require.Equal(t, 1, len(dirents))
				require.Equal(t, "test.txt", dirents[0].Name)
				require.Zero(t, dirents[0].Type)
			})
		})
	}
}

func testReaddirAll(t *testing.T, dotF sys.File, expectDirIno bool) {
	dirents, errno := dotF.Readdir(-1)
	require.EqualErrno(t, 0, errno) // no io.EOF when -1 is used
	sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

	requireIno(t, dirents, expectDirIno)

	// Scrub inodes so we can compare expectations without them.
	for i := range dirents {
		dirents[i].Ino = 0
	}

	require.Equal(t, []sys.Dirent{
		{Name: "animals.txt", Type: 0},
		{Name: "dir", Type: fs.ModeDir},
		{Name: "empty.txt", Type: 0},
		{Name: "emptydir", Type: fs.ModeDir},
		{Name: "sub", Type: fs.ModeDir},
	}, dirents)
}

func requireIno(t *testing.T, dirents []sys.Dirent, expectDirIno bool) {
	for _, e := range dirents {
		if expectDirIno {
			require.NotEqual(t, uint64(0), e.Ino, "%+v", e)
			e.Ino = 0
		} else {
			require.Zero(t, e.Ino, "%+v", e)
		}
	}
}
