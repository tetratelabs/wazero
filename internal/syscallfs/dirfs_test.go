package syscallfs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestDirFS_MkDir(t *testing.T) {
	dir := t.TempDir()

	testFS := dirFS(dir)

	name := "mkdir"
	realPath := path.Join(dir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		require.NoError(t, testFS.Mkdir(name, fs.ModeDir))
		stat, err := os.Stat(realPath)
		require.NoError(t, err)
		require.Equal(t, name, stat.Name())
		require.True(t, stat.IsDir())
	})

	t.Run("dir exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, errors.Unwrap(err))
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.Remove(realPath))
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, errors.Unwrap(err))
	})
}

func TestDirFS_Rename(t *testing.T) {
	t.Run("from doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		err := os.WriteFile(file1Path, []byte{1}, 0o600)
		require.NoError(t, err)

		err = testFS.Rename("file2", file1)
		require.Equal(t, syscall.ENOENT, err)
	})
	t.Run("file to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2 := "file2"
		file2Path := path.Join(tmpDir, file2)
		err = testFS.Rename(file1, file2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		s, err := os.Stat(file2Path)
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("dir to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)
		err := testFS.Rename(dir1, dir2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		s, err := os.Stat(dir2Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("dir to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)

		// write a file to that path
		err := os.WriteFile(dir2Path, []byte{2}, 0o600)
		require.NoError(t, err)

		err = testFS.Rename(dir1, dir2)
		if runtime.GOOS == "windows" {
			require.NoError(t, err)

			// Show the directory moved
			s, err := os.Stat(dir2Path)
			require.NoError(t, err)
			require.True(t, s.IsDir())
		} else {
			require.Equal(t, syscall.ENOTDIR, err)
		}
	})
	t.Run("file to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err = testFS.Rename(file1, dir1)
		require.Equal(t, syscall.EISDIR, err)
	})
	t.Run("dir to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := path.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		err = testFS.Rename(dir1, dir2)
		if runtime.GOOS == "windows" {
			// Windows doesn't let you overwrite an existing directory.
			require.Equal(t, syscall.EINVAL, err)
			return
		}
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		// Show the file inside that directory moved
		s, err := os.Stat(path.Join(dir2Path, file1))
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("file to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2 := "file2"
		file2Path := path.Join(tmpDir, file2)
		file2Contents := []byte{2}
		err = os.WriteFile(file2Path, file2Contents, 0o600)
		require.NoError(t, err)

		err = testFS.Rename(file1, file2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		// Show the file1 overwrote file2
		b, err := os.ReadFile(file2Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
	t.Run("dir to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err := testFS.Rename(dir1, dir1)
		require.NoError(t, err)

		s, err := os.Stat(dir1Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("file to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := dirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		err = testFS.Rename(file1, file1)
		require.NoError(t, err)

		b, err := os.ReadFile(file1Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
}

func TestDirFS_Rmdir(t *testing.T) {
	dir := t.TempDir()

	testFS := dirFS(dir)

	name := "rmdir"
	realPath := path.Join(dir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir not empty", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))
		fileInDir := path.Join(realPath, "file")
		require.NoError(t, os.WriteFile(fileInDir, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTEMPTY, err)

		require.NoError(t, os.Remove(fileInDir))
	})

	t.Run("dir empty", func(t *testing.T) {
		require.NoError(t, testFS.Rmdir(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})

	t.Run("not directory", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTDIR, err)

		require.NoError(t, os.Remove(realPath))
	})
}

func TestDirFS_Unlink(t *testing.T) {
	dir := t.TempDir()

	testFS := dirFS(dir)

	name := "unlink"
	realPath := path.Join(dir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Unlink(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("not file", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Unlink(name)
		require.Equal(t, syscall.EISDIR, err)

		require.NoError(t, os.Remove(realPath))
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		require.NoError(t, testFS.Unlink(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})
}

func TestDirFS_Utimes(t *testing.T) {
	tmpDir := t.TempDir()

	testFS := dirFS(tmpDir)

	file := "file"
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Utimes("nope",
			time.Unix(123, 4*1e3).UnixNano(),
			time.Unix(567, 8*1e3).UnixNano())
		require.Equal(t, syscall.ENOENT, err)
	})

	type test struct {
		name                 string
		path                 string
		atimeNsec, mtimeNsec int64
	}

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{
			name:      "file positive",
			path:      file,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{
			name:      "dir positive",
			path:      dir,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "file zero", path: file},
		{name: "dir zero", path: dir},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{
				name:      "file negative",
				path:      file,
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
			test{
				name:      "dir negative",
				path:      dir,
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := testFS.Utimes(tc.path, tc.atimeNsec, tc.mtimeNsec)
			require.NoError(t, err)

			stat, err := os.Stat(path.Join(tmpDir, tc.path))
			require.NoError(t, err)

			atimeNsec, mtimeNsec, _ := platform.StatTimes(stat)
			if platform.CompilerSupported() {
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}
