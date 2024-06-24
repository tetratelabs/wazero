package sysfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReadLinkAt(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("simple link", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "dir0")
		linkPath := filepath.Join(dirPath, "link")
		linkContent := "file"

		err := os.MkdirAll(dirPath, 0o700)
		require.NoError(t, err)
		dir, err := os.Open(dirPath)
		require.NoError(t, err)
		defer dir.Close()

		err = os.Symlink(linkContent, linkPath)
		require.NoError(t, err)

		content, err := ReadLinkAt(dir, "link")
		require.NoError(t, err)
		require.Equal(t, linkContent, content)
	})
}
