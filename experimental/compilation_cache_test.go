package experimental

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/compilationcache"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

func TestWithCompilationCacheDirName(t *testing.T) {
	ctx := context.WithValue(context.Background(), version.WazeroVersionKey{}, "dev")
	// We expect to create a version-specific subdirectory.
	expectedSubdir := fmt.Sprintf("wazero-dev-%s-%s", runtime.GOARCH, runtime.GOOS)

	t.Run("ok", func(t *testing.T) {
		dir := t.TempDir()
		ctx, err := WithCompilationCacheDirName(ctx, dir)
		require.NoError(t, err)
		actual, ok := ctx.Value(compilationcache.FileCachePathKey{}).(string)
		require.True(t, ok)
		require.Equal(t, path.Join(dir, expectedSubdir), actual)

		// Ensure that the sanity check file has been removed.
		entries, err := os.ReadDir(actual)
		require.NoError(t, err)
		require.Equal(t, 0, len(entries))
	})
	t.Run("create dir", func(t *testing.T) {
		tmpDir := path.Join(t.TempDir(), "1", "2", "3")
		dir := path.Join(tmpDir, "foo") // Non-existent directory.
		absDir, err := filepath.Abs(dir)
		require.NoError(t, err)

		ctx, err := WithCompilationCacheDirName(ctx, dir)
		require.NoError(t, err)
		actual, ok := ctx.Value(compilationcache.FileCachePathKey{}).(string)
		require.True(t, ok)
		require.Equal(t, path.Join(absDir, expectedSubdir), actual)

		requireContainsDir(t, tmpDir, "foo", actual)
	})
	t.Run("create relative dir", func(t *testing.T) {
		tmpDir, oldwd := requireChdirToTemp(t)
		defer os.Chdir(oldwd) //nolint
		dir := "foo"
		absDir, err := filepath.Abs(dir)
		require.NoError(t, err)

		ctx, err := WithCompilationCacheDirName(ctx, dir)
		require.NoError(t, err)
		actual, ok := ctx.Value(compilationcache.FileCachePathKey{}).(string)
		require.True(t, ok)
		require.Equal(t, path.Join(absDir, expectedSubdir), actual)

		requireContainsDir(t, tmpDir, dir, actual)
	})
	t.Run("basedir is not a dir", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "nondir")
		require.NoError(t, err)
		defer f.Close()

		_, err = WithCompilationCacheDirName(ctx, f.Name())
		require.Contains(t, err.Error(), "is not dir")
	})
	t.Run("versiondir is not a dir", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(path.Join(dir, expectedSubdir), []byte{}, 0o600))

		_, err := WithCompilationCacheDirName(ctx, dir)
		require.Contains(t, err.Error(), "is not dir")
	})
}

// requireContainsDir ensures the directory was created in the correct path,
// as file.Abs can return slightly different answers for a temp directory. For
// example, /var/folders/... vs /private/var/folders/...
func requireContainsDir(t *testing.T, parent, dir string, actual string) {
	require.True(t, filepath.IsAbs(actual))

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries))
	require.Equal(t, dir, entries[0].Name())
	require.True(t, entries[0].IsDir())
}

func requireChdirToTemp(t *testing.T) (string, string) {
	tmpDir := t.TempDir()
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	return tmpDir, oldwd
}
