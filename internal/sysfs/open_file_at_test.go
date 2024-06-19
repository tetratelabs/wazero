package sysfs

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestOpenFileAt(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("file under a directory", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir0")
		fileName := "file"
		filePath := filepath.Join(dirPath, fileName)

		err := os.Mkdir(dirPath, 0o777)
		require.NoError(t, err)

		f, err := os.Create(filePath)
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		file, errno := OpenFileAt(dir, fileName, sys.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("create new file under a directory", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir1")
		fileName := "file"

		err := os.Mkdir(dirPath, 0o700)
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		file, errno := OpenFileAt(dir, fileName, sys.O_RDWR|sys.O_CREAT, 0)
		require.EqualErrno(t, 0, errno)
		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("create new file under a nested directory", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir2")
		nestedDirName := "dir"
		nestedDirPath := filepath.Join(dirPath, nestedDirName)
		fileName := "file"

		err := os.MkdirAll(nestedDirPath, 0o700)
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		file, errno := OpenFileAt(dir, path.Join(nestedDirName, fileName), sys.O_RDWR|sys.O_CREAT, 0o700)
		require.EqualErrno(t, 0, errno)
		err = file.Close()
		require.NoError(t, err)

		file, err = os.Open(filepath.Join(nestedDirPath, fileName))
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("create file in parent", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir3")
		fileName := "file"
		filePath := filepath.Join(tempDir, fileName)

		err := os.MkdirAll(dirPath, 0o700)
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		file, errno := OpenFileAt(dir, path.Join("..", fileName), sys.O_RDWR|sys.O_CREAT, 0o700)
		require.EqualErrno(t, 0, errno)
		err = file.Close()
		require.NoError(t, err)

		file, err = os.Open(filePath)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("resolve dot dot", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir4")
		nestedDirName := "dir"
		nestedDirPath := filepath.Join(dirPath, nestedDirName)
		fileName := "file"
		filePath := filepath.Join(dirPath, fileName)

		err := os.MkdirAll(nestedDirPath, 0o700)
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		file, errno := OpenFileAt(dir, path.Join(".", nestedDirName, "..", ".", fileName), sys.O_RDWR|sys.O_CREAT, 0o700)
		require.EqualErrno(t, 0, errno)
		err = file.Close()
		require.NoError(t, err)

		file, err = os.Open(filePath)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)
	})

	t.Run("no follow symlink", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir5")
		fileName := "file"
		linkName := "link"
		filePath := filepath.Join(dirPath, fileName)
		linkPath := filepath.Join(dirPath, linkName)

		err := os.MkdirAll(dirPath, 0o700)
		require.NoError(t, err)

		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		f, err := os.Create(filePath)
		require.NoError(t, err)
		err = f.Close()
		require.NoError(t, err)

		err = os.Symlink(fileName, linkPath)
		require.NoError(t, err)

		_, err = OpenFileAt(dir, linkName, sys.O_RDONLY|sys.O_NOFOLLOW, 0o700)
		require.EqualErrno(t, sys.ELOOP, err)
	})
}
