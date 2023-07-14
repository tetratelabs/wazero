package sysfs

import (
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUnlink(t *testing.T) {
	t.Run("doesn't exist", func(t *testing.T) {
		name := "non-existent"
		errno := unlink(name)
		require.EqualErrno(t, sys.ENOENT, errno)
	})

	t.Run("target: dir", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir := path.Join(tmpDir, "dir")
		require.NoError(t, os.Mkdir(dir, 0o700))

		errno := unlink(dir)
		require.EqualErrno(t, sys.EISDIR, errno)

		require.NoError(t, os.Remove(dir))
	})

	t.Run("target: symlink to dir", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create link target dir.
		subDirRealPath := path.Join(tmpDir, "subdir")
		require.NoError(t, os.Mkdir(subDirRealPath, 0o700))

		// Create a symlink to the subdirectory.
		const symlinkName = "symlink-to-dir"
		require.NoError(t, os.Symlink("subdir", symlinkName))

		// Unlinking the symlink should suceed.
		errno := unlink(symlinkName)
		require.EqualErrno(t, 0, errno)
	})

	t.Run("file exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		name := path.Join(tmpDir, "unlink")

		require.NoError(t, os.WriteFile(name, []byte{}, 0o600))

		require.EqualErrno(t, 0, unlink(name))
		_, err := os.Stat(name)
		require.Error(t, err)
	})
}
