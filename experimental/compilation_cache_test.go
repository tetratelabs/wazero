package experimental

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/compilationcache"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWithCompilationCacheDirName(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		dir := t.TempDir()
		ctx, err := WithCompilationCacheDirName(context.Background(), dir)
		require.NoError(t, err)
		actual, ok := ctx.Value(compilationcache.FileCachePathKey{}).(string)
		require.True(t, ok)
		require.Equal(t, dir, actual)

		// Ensure that the sanity check file has been removed.
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Equal(t, 0, len(entries))
	})
	t.Run("create dir", func(t *testing.T) {
		dir := path.Join(t.TempDir(), "1", "2", "3", t.Name()) // Non-existent directory.
		fmt.Println(dir)
		ctx, err := WithCompilationCacheDirName(context.Background(), dir)
		require.NoError(t, err)
		actual, ok := ctx.Value(compilationcache.FileCachePathKey{}).(string)
		require.True(t, ok)
		require.Equal(t, dir, actual)
	})
	t.Run("non dir", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "nondir")
		require.NoError(t, err)
		defer f.Close()

		_, err = WithCompilationCacheDirName(context.Background(), f.Name())
		require.Contains(t, err.Error(), "is not dir")
	})
}
