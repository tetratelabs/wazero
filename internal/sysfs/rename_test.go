package sysfs

import (
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestRename(t *testing.T) {
	t.Run("from doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1Path := path.Join(tmpDir, "file1")
		err := os.WriteFile(file1Path, []byte{1}, 0o600)
		require.NoError(t, err)

		err = rename(path.Join(tmpDir, "non-exist"), file1Path)
		require.EqualErrno(t, sys.ENOENT, err)
	})
	t.Run("file to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1Path := path.Join(tmpDir, "file1")
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2Path := path.Join(tmpDir, "file2")
		errno := rename(file1Path, file2Path)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		s, err := os.Stat(file2Path)
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("dir to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1Path := path.Join(tmpDir, "dir1")
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2Path := path.Join(tmpDir, "dir2")
		errno := rename(dir1Path, dir2Path)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err := os.Stat(dir1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		s, err := os.Stat(dir2Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("dir to file", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1Path := path.Join(tmpDir, "dir1")
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2Path := path.Join(tmpDir, "dir2")

		// write a file to that path
		err := os.WriteFile(dir2Path, []byte{2}, 0o600)
		require.NoError(t, err)

		err = rename(dir1Path, dir2Path)
		require.EqualErrno(t, sys.ENOTDIR, err)
	})
	t.Run("file to dir", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1Path := path.Join(tmpDir, "file1")
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir1Path := path.Join(tmpDir, "dir1")
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err = rename(file1Path, dir1Path)
		require.EqualErrno(t, sys.EISDIR, err)
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L567-L582
	t.Run("dir to empty dir should be fine", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := path.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2Path := path.Join(tmpDir, "dir2")
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		errno := rename(dir1Path, dir2Path)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		// Show the file inside that directory moved
		s, err := os.Stat(path.Join(dir2Path, file1))
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L584-L604
	t.Run("dir to non empty dir should be EXIST", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1Path := path.Join(dir1Path, "file1")
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2Path := path.Join(tmpDir, "dir2")
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		// Make the destination non-empty.
		err = os.WriteFile(path.Join(dir2Path, "existing.txt"), []byte("any thing"), 0o600)
		require.NoError(t, err)

		err = rename(dir1Path, dir2Path)
		require.EqualErrno(t, sys.ENOTEMPTY, err)
	})

	t.Run("file to file", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1Path := path.Join(tmpDir, "file1")
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2Path := path.Join(tmpDir, "file2")
		file2Contents := []byte{2}
		err = os.WriteFile(file2Path, file2Contents, 0o600)
		require.NoError(t, err)

		errno := rename(file1Path, file2Path)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		// Show the file1 overwrote file2
		b, err := os.ReadFile(file2Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
	t.Run("dir to itself", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1Path := path.Join(tmpDir, "dir1")
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		errno := rename(dir1Path, dir1Path)
		require.EqualErrno(t, 0, errno)

		s, err := os.Stat(dir1Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("file to itself", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1Path := path.Join(tmpDir, "file1")
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		errno := rename(file1Path, file1Path)
		require.EqualErrno(t, 0, errno)

		b, err := os.ReadFile(file1Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
}
