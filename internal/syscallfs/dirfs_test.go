package syscallfs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"

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
		err := testFS.Utimes("nope", 123, 4, 567, 8)
		require.Equal(t, syscall.ENOENT, err)
	})

	type test struct {
		name                                     string
		path                                     string
		atimeSec, atimeNsec, mtimeSec, mtimeNsec int64
	}

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{name: "file positive", path: file, atimeSec: 123, atimeNsec: 4 * 1e3, mtimeSec: 567, mtimeNsec: 8 * 1e3},
		{name: "dir positive", path: dir, atimeSec: 123, atimeNsec: 4 * 1e3, mtimeSec: 567, mtimeNsec: 8 * 1e3},
		{name: "file zero", path: file},
		{name: "dir zero", path: dir},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{name: "file negative", path: file, atimeSec: -123, atimeNsec: -4 * 1e3, mtimeSec: -567, mtimeNsec: -8 * 1e3},
			test{name: "dir negative", path: dir, atimeSec: -123, atimeNsec: -4 * 1e3, mtimeSec: -567, mtimeNsec: -8 * 1e3},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := testFS.Utimes(tc.path, tc.atimeSec, tc.atimeNsec, tc.mtimeSec, tc.mtimeNsec)
			require.NoError(t, err)

			stat, err := os.Stat(path.Join(tmpDir, tc.path))
			require.NoError(t, err)

			atimeSec, atimeNsec, mtimeSec, mtimeNsec, _, _ := platform.StatTimes(stat)
			if platform.CompilerSupported() {
				require.Equal(t, atimeSec, tc.atimeSec)
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeSec, tc.mtimeSec)
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}
