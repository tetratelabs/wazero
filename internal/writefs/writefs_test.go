package writefs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var testFiles = map[string]string{
	"empty.txt":        "",
	"test.txt":         "animals\n",
	"sub/test.txt":     "greet sub dir\n",
	"sub/sub/test.txt": "greet sub sub dir\n",
}

func TestDirFS_TestFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This abstraction is a toe-hold, but we'll have to sort windows with
		// our ideal filesystem tester.
		t.Skip("TODO: windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(path.Join(dir, "sub", "sub"), 0o700))

	expected := make([]string, 0, len(testFiles))
	for name, data := range testFiles {
		expected = append(expected, name)
		require.NoError(t, os.WriteFile(path.Join(dir, name), []byte(data), 0o600))
	}

	if err := fstest.TestFS(DirFS(dir), expected...); err != nil {
		t.Fatal(err)
	}
}

func TestDirFS_MkDir(t *testing.T) {
	dir := t.TempDir()

	testFS := DirFS(dir)

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

	testFS := DirFS(dir)

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

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTDIR, err)

		require.NoError(t, os.Remove(realPath))
	})
}

func TestDirFS_Unlink(t *testing.T) {
	dir := t.TempDir()

	testFS := DirFS(dir)

	name := "unlink"
	realPath := path.Join(dir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Unlink(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir exists", func(t *testing.T) {
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
