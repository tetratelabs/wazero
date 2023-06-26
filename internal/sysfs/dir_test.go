package sysfs_test

import (
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReaddir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

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
			dotF, errno := sysfs.OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer dotF.Close()

			t.Run("dir", func(t *testing.T) {
				dirs, errno := dotF.Readdir()
				require.EqualErrno(t, 0, errno)
				defer dirs.Close()

				testReaddirAll(t, dirs, tc.expectIno)

				// read again even though it is exhausted
				dirents, errno := sysfs.ReaddirAll(dirs)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, len(dirents))

				// Reset to the initial position (Rewind to zero)
				errno = dirs.Rewind(0)
				require.EqualErrno(t, 0, errno)

				// We should be able to read again
				testReaddirAll(t, dirs, tc.expectIno)
			})

			// Don't err if something else closed the directory while reading.
			t.Run("closed dir", func(t *testing.T) {
				dotF, errno := sysfs.OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				defer dotF.Close()

				require.EqualErrno(t, 0, dotF.Close())
				dirs, errno := dotF.Readdir()
				defer dirs.Close()
				require.EqualErrno(t, 0, errno)
			})

			fileF, errno := sysfs.OpenFSFile(tc.fs, "empty.txt", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				dir, errno := fileF.Readdir()
				defer dir.Close()
				require.EqualErrno(t, syscall.ENOTDIR, errno)
			})

			dirF, errno := sysfs.OpenFSFile(tc.fs, "dir", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer dirF.Close()

			t.Run("partial", func(t *testing.T) {
				dirs, errno := dirF.Readdir()
				defer dirs.Close()
				require.EqualErrno(t, 0, errno)

				dirent1, errno := dirs.Next()
				require.EqualErrno(t, 0, errno)

				dirent2, errno := dirs.Next()
				require.EqualErrno(t, 0, errno)

				// read exactly the last entry
				dirent3, errno := dirs.Next()
				require.EqualErrno(t, 0, errno)

				dirents := []fsapi.Dirent{*dirent1, *dirent2, *dirent3}
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				requireIno(t, dirents, tc.expectIno)

				// Scrub inodes so we can compare expectations without them.
				for i := range dirents {
					dirents[i].Ino = 0
				}

				require.Equal(t, []fsapi.Dirent{
					{Name: "-", Type: 0},
					{Name: "a-", Type: fs.ModeDir},
					{Name: "ab-", Type: 0},
				}, dirents)

				// no error reading an exhausted directory
				dirs, errno = dirF.Readdir()
				defer dirs.Close()
				require.EqualErrno(t, 0, errno)
			})

			subdirF, errno := sysfs.OpenFSFile(tc.fs, "sub", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer subdirF.Close()

			t.Run("subdir", func(t *testing.T) {
				dirs, errno := subdirF.Readdir()
				defer dirs.Close()
				require.EqualErrno(t, 0, errno)
				dirents, errno := sysfs.ReaddirAll(dirs)

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
		dirF, errno := sysfs.OpenFSFile(dirFS, "dir", syscall.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer dirF.Close()

		dirs, errno := dirF.Readdir()
		defer dirs.Close()
		require.EqualErrno(t, 0, errno)

		_, errno = dirs.Next()
		require.EqualErrno(t, 0, errno)

		// Speculatively try to remove even if it won't likely work
		// on windows.
		err := os.RemoveAll(path.Join(tmpDir, "dir"))
		if err != nil && runtime.GOOS == "windows" {
			t.Skip()
		} else {
			require.NoError(t, err)
		}

		dirs2, errno := dirF.Readdir()
		defer dirs2.Close()
		require.EqualErrno(t, 0, errno)
		// don't validate the contents as due to caching it might be present.
	})
}

func testReaddirAll(t *testing.T, dirs fsapi.Readdir, expectIno bool) {
	dirents, errno := sysfs.ReaddirAll(dirs)
	require.EqualErrno(t, 0, errno)
	sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

	requireIno(t, dirents, expectIno)

	// Scrub inodes so we can compare expectations without them.
	for i := range dirents {
		dirents[i].Ino = 0
	}

	require.Equal(t, []fsapi.Dirent{
		{Name: "animals.txt", Type: 0},
		{Name: "dir", Type: fs.ModeDir},
		{Name: "empty.txt", Type: 0},
		{Name: "emptydir", Type: fs.ModeDir},
		{Name: "sub", Type: fs.ModeDir},
	}, dirents)
}

func requireIno(t *testing.T, dirents []fsapi.Dirent, expectIno bool) {
	for _, e := range dirents {
		if expectIno {
			require.NotEqual(t, uint64(0), e.Ino, "%+v", e)
			e.Ino = 0
		} else {
			require.Zero(t, e.Ino, "%+v", e)
		}
	}
}
