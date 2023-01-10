package syscallfs

import (
	"errors"
	"io/fs"
	"os"
	pathutil "path"
	"runtime"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestDirFS_MkDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	name := "mkdir"
	realPath := pathutil.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		require.NoError(t, testFS.Mkdir(name, fs.ModeDir))
		stat, err := os.Stat(realPath)
		require.NoError(t, err)
		require.Equal(t, name, stat.Name())
		require.True(t, stat.IsDir())
	})

	t.Run("dir exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		requireErrno(t, syscall.EEXIST, err)
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.Remove(realPath))
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Mkdir(name, fs.ModeDir)
		requireErrno(t, syscall.EEXIST, err)
	})
}

func TestDirFS_Rename(t *testing.T) {
	t.Run("from doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		err = os.WriteFile(file1Path, []byte{1}, 0o600)
		require.NoError(t, err)

		err = testFS.Rename("file2", file1)
		require.Equal(t, syscall.ENOENT, err)
	})
	t.Run("file to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err = os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2 := "file2"
		file2Path := pathutil.Join(tmpDir, file2)
		err = testFS.Rename(file1, file2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		requireErrno(t, syscall.ENOENT, err)

		s, err := os.Stat(file2Path)
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("dir to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)
		err = testFS.Rename(dir1, dir2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		requireErrno(t, syscall.ENOENT, err)

		s, err := os.Stat(dir2Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("dir to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)

		// write a file to that path
		err = os.WriteFile(dir2Path, []byte{2}, 0o600)
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
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err = os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err = testFS.Rename(file1, dir1)
		require.Equal(t, syscall.EISDIR, err)
	})
	t.Run("dir to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := pathutil.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err = os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)
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
		s, err := os.Stat(pathutil.Join(dir2Path, file1))
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("file to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err = os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2 := "file2"
		file2Path := pathutil.Join(tmpDir, file2)
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
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err = testFS.Rename(dir1, dir1)
		require.NoError(t, err)

		s, err := os.Stat(dir1Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("file to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS, err := NewDirFS(tmpDir)
		require.NoError(t, err)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err = os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		err = testFS.Rename(file1, file1)
		require.NoError(t, err)

		b, err := os.ReadFile(file1Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
}

func TestDirFS_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()

	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	name := "rmdir"
	realPath := pathutil.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir not empty", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))
		fileInDir := pathutil.Join(realPath, "file")
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
	tmpDir := t.TempDir()

	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	name := "unlink"
	realPath := pathutil.Join(tmpDir, name)

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

	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	testUtimes(t, tmpDir, testFS)
}

func TestDirFS_Open(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = pathutil.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))

	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	testOpen_Read(t, tmpDir, testFS)

	testOpen_O_RDWR(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// syscall.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestDirFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	// Create a writeable filesystem
	testFS, err := NewDirFS(tmpDir)
	require.NoError(t, err)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(&testFSAdapter{testFS}))
}
