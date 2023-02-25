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
			dirF, err := tc.fs.Open(".")
			require.NoError(t, err)
			defer dirF.Close()

			t.Run("dir", func(t *testing.T) {
				names, err := platform.Readdirnames(dirF, -1)
				require.NoError(t, err)
				sort.Strings(names)
				require.Equal(t, []string{"animals.txt", "dir", "empty.txt", "emptydir", "sub"}, names)

				// read again even though it is exhausted
				_, err = platform.Readdirnames(dirF, 100)
				require.EqualErrno(t, syscall.EIO, err)
			})

			// windows and fstest.MapFS allow you to read a closed dir
			if runtime.GOOS != "windows" && tc.name != "fstest.MapFS" {
				t.Run("closed dir", func(t *testing.T) {
					require.NoError(t, dirF.Close())
					_, err := platform.Readdirnames(dirF, -1)
					require.EqualErrno(t, syscall.EIO, err)
				})
			}

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
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},         // To test readdirFile
		{name: "fstest.MapFS", fs: fstest.FS}, // To test adaptation of ReadDirFile
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			dirF, err := tc.fs.Open(".")
			require.NoError(t, err)
			defer dirF.Close()

			t.Run("dir", func(t *testing.T) {
				dirents, err := platform.Readdir(dirF, -1)
				require.NoError(t, err) // no io.EOF when -1 is used
				sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

				require.Equal(t, []*platform.Dirent{
					{Name: "animals.txt", Type: 0},
					{Name: "dir", Type: fs.ModeDir},
					{Name: "empty.txt", Type: 0},
					{Name: "emptydir", Type: fs.ModeDir},
					{Name: "sub", Type: fs.ModeDir},
				}, dirents)

				// read again even though it is exhausted
				dirents, err = platform.Readdir(dirF, 100)
				require.Equal(t, io.EOF, err)
				require.Zero(t, len(dirents))
			})

			// Don't err if something else closed the directory while reading.
			t.Run("closed dir", func(t *testing.T) {
				require.NoError(t, dirF.Close())
				_, err := platform.Readdir(dirF, -1)
				require.NoError(t, err)
			})

			fileF, err := tc.fs.Open("empty.txt")
			require.NoError(t, err)
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, err := platform.Readdir(fileF, -1)
				require.EqualErrno(t, syscall.ENOTDIR, err)
			})

			dirF, err = tc.fs.Open("dir")
			require.NoError(t, err)
			defer dirF.Close()

			t.Run("partial read", func(t *testing.T) {
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

				require.Equal(t, []*platform.Dirent{
					{Name: "-", Type: 0},
					{Name: "a-", Type: fs.ModeDir},
					{Name: "ab-", Type: 0},
				}, dirents)

				_, err = platform.Readdir(dirF, 1)
				require.Equal(t, io.EOF, err)
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
