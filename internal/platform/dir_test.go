package platform_test

import (
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReaddir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name      string
		fs        fs.FS
		expectIno bool
	}{
		{name: "os.DirFS", fs: dirFS, expectIno: runtime.GOOS != "windows"}, // To test readdirFile
		{name: "fstest.MapFS", fs: fstest.FS, expectIno: false},             // To test adaptation of ReadDirFile
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			dotF, errno := platform.OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
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

				// We should be able to read again
				testReaddirAll(t, dotF, tc.expectIno)
			})

			// Don't err if something else closed the directory while reading.
			t.Run("closed dir", func(t *testing.T) {
				require.EqualErrno(t, 0, dotF.Close())
				_, errno := dotF.Readdir(-1)
				require.EqualErrno(t, 0, errno)
			})

			fileF, errno := platform.OpenFSFile(tc.fs, "empty.txt", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, errno := fileF.Readdir(-1)
				require.EqualErrno(t, syscall.ENOTDIR, errno)
			})

			dirF, errno := platform.OpenFSFile(tc.fs, "dir", syscall.O_RDONLY, 0)
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

				dirents := []platform.Dirent{dirents1[0], dirents2[0], dirents3[0]}
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				requireIno(t, dirents, tc.expectIno)

				// Scrub inodes so we can compare expectations without them.
				for i := range dirents {
					dirents[i].Ino = 0
				}

				require.Equal(t, []platform.Dirent{
					{Name: "-", Type: 0},
					{Name: "a-", Type: fs.ModeDir},
					{Name: "ab-", Type: 0},
				}, dirents)

				// no error reading an exhausted directory
				_, errno = dirF.Readdir(1)
				require.EqualErrno(t, 0, errno)
			})

			subdirF, errno := platform.OpenFSFile(tc.fs, "sub", syscall.O_RDONLY, 0)
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

	// Don't err if something else removed the directory while reading.
	t.Run("removed while open", func(t *testing.T) {
		dirF, errno := platform.OpenFSFile(dirFS, "dir", syscall.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer dirF.Close()

		dirents, errno := dirF.Readdir(1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents))

		// Speculatively try to remove even if it won't likely work
		// on windows.
		err := os.RemoveAll(path.Join(tmpDir, "dir"))
		if err != nil && runtime.GOOS == "windows" {
			t.Skip()
		} else {
			require.NoError(t, err)
		}

		_, errno = dirF.Readdir(1)
		require.EqualErrno(t, 0, errno)
		// don't validate the contents as due to caching it might be present.
	})
}

func testReaddirAll(t *testing.T, dotF platform.File, expectIno bool) {
	dirents, errno := dotF.Readdir(-1)
	require.EqualErrno(t, 0, errno) // no io.EOF when -1 is used
	sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

	requireIno(t, dirents, expectIno)

	// Scrub inodes so we can compare expectations without them.
	for i := range dirents {
		dirents[i].Ino = 0
	}

	require.Equal(t, []platform.Dirent{
		{Name: "animals.txt", Type: 0},
		{Name: "dir", Type: fs.ModeDir},
		{Name: "empty.txt", Type: 0},
		{Name: "emptydir", Type: fs.ModeDir},
		{Name: "sub", Type: fs.ModeDir},
	}, dirents)
}

func requireIno(t *testing.T, dirents []platform.Dirent, expectIno bool) {
	for _, e := range dirents {
		if expectIno {
			require.NotEqual(t, uint64(0), e.Ino, "%+v", e)
			e.Ino = 0
		} else {
			require.Zero(t, e.Ino, "%+v", e)
		}
	}
}
