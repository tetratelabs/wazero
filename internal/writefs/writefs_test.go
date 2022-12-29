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

func TestFS(t *testing.T) {
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

func TestMkDir(t *testing.T) {
	dir := t.TempDir()

	testFS := DirFS(dir)

	name := "mkdir"

	t.Run("doesn't exist", func(t *testing.T) {
		require.NoError(t, testFS.Mkdir(name, fs.ModeDir))
		stat, err := os.Stat(path.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, name, stat.Name())
		require.True(t, stat.IsDir())
	})

	t.Run("dir exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, errors.Unwrap(err))
	})

	t.Run("file exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, errors.Unwrap(err))
	})
}

func TestRmdir(t *testing.T) {
	dir := t.TempDir()

	testFS := DirFS(dir)

	name := "rmdir"

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir exists", func(t *testing.T) {
		realPath := path.Join(dir, name)
		err := os.Mkdir(realPath, 0o700)
		require.NoError(t, err)

		require.NoError(t, testFS.Rmdir(name))
		_, err = os.Stat(realPath)
		require.Error(t, err)
	})

	t.Run("file exists", func(t *testing.T) {
		realPath := path.Join(dir, name)
		err := os.WriteFile(realPath, []byte{}, 0o600)
		require.NoError(t, err)

		err = testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTDIR, err)

		require.NoError(t, os.Remove(realPath))
	})
}

func TestUnlink(t *testing.T) {
	dir := t.TempDir()

	testFS := DirFS(dir)

	name := "unlink"

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Unlink(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir exists", func(t *testing.T) {
		realPath := path.Join(dir, name)
		err := os.Mkdir(realPath, 0o700)
		require.NoError(t, err)

		err = testFS.Unlink(name)
		require.Equal(t, syscall.EISDIR, err)

		require.NoError(t, os.Remove(realPath))
	})

	t.Run("file exists", func(t *testing.T) {
		realPath := path.Join(dir, name)
		err := os.WriteFile(realPath, []byte{}, 0o600)
		require.NoError(t, err)

		require.NoError(t, testFS.Unlink(name))
		_, err = os.Stat(realPath)
		require.Error(t, err)
	})
}
