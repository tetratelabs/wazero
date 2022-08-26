package compilationcache

import (
	"bytes"
	"io"
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFileReadCloser_Close(t *testing.T) {
	fc := newFileCache(t.TempDir())
	key := Key{1, 2, 3}

	err := fc.Add(key, bytes.NewReader([]byte{1, 2, 3, 4}))
	require.NoError(t, err)

	c, ok, err := fc.Get(key)
	require.NoError(t, err)
	require.True(t, ok)

	// At this point, file is not closed, therefore TryLock should fail.
	require.False(t, fc.mux.TryLock())

	// Close, and then TryLock should succeed this time.
	require.NoError(t, c.Close())
	require.True(t, fc.mux.TryLock())
}

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

func TestFileCache_dirPath(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := path.Join(tmp, "test")
	id := Key{1, 2, 3}

	t.Run("Get and Delete ok when not exist", func(t *testing.T) {
		fc := newFileCache(cacheDir)

		// Get doesn't eagerly create the directory
		content, ok, err := fc.Get(id)
		require.Nil(t, content)
		require.False(t, ok)
		require.NoError(t, err)
		_, err = os.Open(fc.dirPath)
		require.ErrorIs(t, err, os.ErrNotExist)

		// Delete doesn't err when the directory doesn't exist
		err = fc.Delete(id)
		require.NoError(t, err)
		_, err = os.Open(fc.dirPath)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	content := []byte{1, 2, 3, 4, 5}

	t.Run("Add fails when not a dir", func(t *testing.T) {
		fc := newFileCache(cacheDir)

		f, err := os.Create(cacheDir) // file not dir
		require.NoError(t, err)

		err = fc.Add(id, bytes.NewReader(content))
		require.Contains(t, err.Error(), "fileCache: expected dir")

		// Ensure cleanup
		require.NoError(t, f.Close())
		require.NoError(t, os.Remove(cacheDir))
	})

	t.Run("Add creates dir", func(t *testing.T) {
		fc := newFileCache(cacheDir)

		err := fc.Add(id, bytes.NewReader(content))
		require.NoError(t, err)

		// Ensure we can read the cached entry
		f, err := os.Open(fc.path(id))
		require.NoError(t, err)
		require.NoError(t, f.Close())
	})
}
