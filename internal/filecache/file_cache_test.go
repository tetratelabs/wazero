package filecache

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFileCache_Add(t *testing.T) {
	fc := newFileCache(t.TempDir())

	t.Run("not exist", func(t *testing.T) {
		content := []byte{1, 2, 3, 4, 5}
		id := Key{1, 2, 3, 4, 5, 6, 7}
		err := fc.Add(id, bytes.NewReader(content))
		require.NoError(t, err)

		// Ensures that file exists.
		cached, err := os.ReadFile(fc.path(id))
		require.NoError(t, err)

		// Check if the saved content is the same as the given one.
		require.Equal(t, content, cached)
	})

	t.Run("already exists", func(t *testing.T) {
		content := []byte{1, 2, 3, 4, 5}

		id := Key{1, 2, 3}

		// Writes the pre-existing file for the same ID.
		p := fc.path(id)
		f, err := os.Create(p)
		require.NoError(t, err)
		_, err = f.Write(content)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		err = fc.Add(id, bytes.NewReader(content))
		require.NoError(t, err)

		// Ensures that file exists.
		cached, err := os.ReadFile(fc.path(id))
		require.NoError(t, err)

		// Check if the saved content is the same as the given one.
		require.Equal(t, content, cached)
	})
}

func TestFileCache_Delete(t *testing.T) {
	fc := newFileCache(t.TempDir())
	t.Run("non-exist", func(t *testing.T) {
		id := Key{0}
		err := fc.Delete(id)
		require.NoError(t, err)
	})
	t.Run("exist", func(t *testing.T) {
		id := Key{1, 2, 3}
		p := fc.path(id)
		f, err := os.Create(p)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Ensures that file exists now.
		f, err = os.Open(p)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Delete the cache.
		err = fc.Delete(id)
		require.NoError(t, err)

		// Ensures that file no longer exists.
		_, err = os.Open(p)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestFileCache_Get(t *testing.T) {
	fc := newFileCache(t.TempDir())

	t.Run("exist", func(t *testing.T) {
		content := []byte{1, 2, 3, 4, 5}
		id := Key{1, 2, 3}

		// Writes the pre-existing file for the ID.
		p := fc.path(id)
		f, err := os.Create(p)
		require.NoError(t, err)
		_, err = f.Write(content)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		result, ok, err := fc.Get(id)
		require.NoError(t, err)
		require.True(t, ok)
		defer func() {
			require.NoError(t, result.Close())
		}()

		actual, err := io.ReadAll(result)
		require.NoError(t, err)

		require.Equal(t, content, actual)
	})
	t.Run("not exist", func(t *testing.T) {
		_, ok, err := fc.Get(Key{0xf})
		// Non-exist should not be error.
		require.NoError(t, err)
		require.False(t, ok)
	})
}

func TestFileCache_path(t *testing.T) {
	fc := &fileCache{dirPath: "/tmp/.wazero"}
	actual := fc.path(Key{1, 2, 3, 4, 5})
	require.Equal(t, "/tmp/.wazero/0102030405000000000000000000000000000000000000000000000000000000", actual)
}
