package platform_test

import (
	"io/fs"
	"os"
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
			if err != nil {
				return
			}
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
			if err != nil {
				return
			}
			defer fileF.Close()

			t.Run("file", func(t *testing.T) {
				_, err := platform.Readdirnames(fileF, -1)
				require.EqualErrno(t, syscall.ENOTDIR, err)
			})

			subdirF, err := tc.fs.Open("sub")
			if err != nil {
				return
			}
			defer subdirF.Close()

			t.Run("subdir", func(t *testing.T) {
				names, err := platform.Readdirnames(subdirF, -1)
				require.NoError(t, err)
				require.Equal(t, []string{"test.txt"}, names)
			})
		})
	}
}
