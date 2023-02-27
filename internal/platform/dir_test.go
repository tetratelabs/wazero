package platform_test

import (
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

func TestReaddirnames(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},         // To test readdirnamesFile
		{name: "fstest.MapFS", fs: fstest.FS}, // To test adaptation of ReadDirFile
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			dotF, err := tc.fs.Open(".")
			require.NoError(t, err)
			defer dotF.Close()

			t.Run("dir", func(t *testing.T) {
				names, err := platform.Readdirnames(dotF, -1)
				require.NoError(t, err)
				sort.Strings(names)
				require.Equal(t, []string{"animals.txt", "dir", "empty.txt", "emptydir", "sub"}, names)

				// read again even though it is exhausted
				_, err = platform.Readdirnames(dotF, 100)
				require.NoError(t, err)
			})

			// Don't err if something else closed the directory while reading.
			t.Run("closed dir", func(t *testing.T) {
				require.NoError(t, dotF.Close())
				_, err := platform.Readdir(dotF, -1)
				require.NoError(t, err)
			})

			dirF, err := tc.fs.Open("dir")
			require.NoError(t, err)
			defer dirF.Close()

			t.Run("partial", func(t *testing.T) {
				names1, err := platform.Readdirnames(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(names1))

				names2, err := platform.Readdirnames(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(names2))

				// read exactly the last entry
				names3, err := platform.Readdirnames(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(names3))

				names := []string{names1[0], names2[0], names3[0]}
				sort.Strings(names)

				require.Equal(t, []string{"-", "a-", "ab-"}, names)

				// no error reading an exhausted directory
				_, err = platform.Readdirnames(dirF, 1)
				require.NoError(t, err)
			})

			fileF, err := tc.fs.Open("empty.txt")
			require.NoError(t, err)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, err := platform.Readdirnames(fileF, -1)
				require.EqualErrno(t, syscall.ENOTDIR, err)
			})

			subdirF, err := tc.fs.Open("sub")
			require.NoError(t, err)
			defer subdirF.Close()

			t.Run("subdir", func(t *testing.T) {
				names, err := platform.Readdirnames(subdirF, -1)
				require.NoError(t, err)
				require.Equal(t, []string{"test.txt"}, names)
			})
		})
	}
}

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
			dotF, err := tc.fs.Open(".")
			require.NoError(t, err)
			defer dotF.Close()

			t.Run("dir", func(t *testing.T) {
				dirents, err := platform.Readdir(dotF, -1)
				require.NoError(t, err) // no io.EOF when -1 is used
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				requireIno(t, dirents, tc.expectIno)

				require.Equal(t, []*platform.Dirent{
					{Name: "animals.txt", Type: 0},
					{Name: "dir", Type: fs.ModeDir},
					{Name: "empty.txt", Type: 0},
					{Name: "emptydir", Type: fs.ModeDir},
					{Name: "sub", Type: fs.ModeDir},
				}, dirents)

				// read again even though it is exhausted
				dirents, err = platform.Readdir(dotF, 100)
				require.NoError(t, err)
				require.Zero(t, len(dirents))
			})

			// Don't err if something else closed the directory while reading.
			t.Run("closed dir", func(t *testing.T) {
				require.NoError(t, dotF.Close())
				_, err := platform.Readdir(dotF, -1)
				require.NoError(t, err)
			})

			fileF, err := tc.fs.Open("empty.txt")
			require.NoError(t, err)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, err := platform.Readdir(fileF, -1)
				require.EqualErrno(t, syscall.ENOTDIR, err)
			})

			dirF, err := tc.fs.Open("dir")
			require.NoError(t, err)
			defer dirF.Close()

			t.Run("partial", func(t *testing.T) {
				dirents1, err := platform.Readdir(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(dirents1))

				dirents2, err := platform.Readdir(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(dirents2))

				// read exactly the last entry
				dirents3, err := platform.Readdir(dirF, 1)
				require.NoError(t, err)
				require.Equal(t, 1, len(dirents3))

				dirents := []*platform.Dirent{dirents1[0], dirents2[0], dirents3[0]}
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				requireIno(t, dirents, tc.expectIno)

				require.Equal(t, []*platform.Dirent{
					{Name: "-", Type: 0},
					{Name: "a-", Type: fs.ModeDir},
					{Name: "ab-", Type: 0},
				}, dirents)

				// no error reading an exhausted directory
				_, err = platform.Readdir(dirF, 1)
				require.NoError(t, err)
			})

			subdirF, err := tc.fs.Open("sub")
			require.NoError(t, err)
			defer subdirF.Close()

			t.Run("subdir", func(t *testing.T) {
				dirents, err := platform.Readdir(subdirF, -1)
				require.NoError(t, err)
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				require.Equal(t, 1, len(dirents))
				require.Equal(t, "test.txt", dirents[0].Name)
				require.Zero(t, dirents[0].Type)
			})
		})
	}

	// Don't err if something else removed the directory while reading.
	t.Run("removed while open", func(t *testing.T) {
		dirF, err := dirFS.Open("dir")
		require.NoError(t, err)
		defer dirF.Close()

		dirents, err := platform.Readdir(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(dirents))

		// Speculatively try to remove even if it won't likely work
		// on windows.
		err = os.RemoveAll(path.Join(tmpDir, "dir"))
		if err != nil && runtime.GOOS == "windows" {
			t.Skip()
		} else {
			require.NoError(t, err)
		}

		_, err = platform.Readdir(dirF, 1)
		require.NoError(t, err)
		// don't validate the contents as due to caching it might be present.
	})
}

func requireIno(t *testing.T, dirents []*platform.Dirent, expectIno bool) {
	for _, e := range dirents {
		if expectIno {
			require.NotEqual(t, uint64(0), e.Ino, "%+v", e)
			e.Ino = 0
		} else {
			require.Zero(t, e.Ino, "%+v", e)
		}
	}
}
