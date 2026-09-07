package filecache

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReadOnly(t *testing.T) {
	dir := t.TempDir()
	rw := newFileCache(dir)
	ro := NewReadOnly(dir)
	key := Key{1}
	require.True(t, ReadOnly(ro))
	require.False(t, ReadOnly(rw))
	require.False(t, ReadOnly(nil))
	_, hit, err := ro.Get(key)
	require.False(t, hit)
	require.True(t, errors.Is(err, ErrMiss))
	require.NoError(t, rw.Add(key, bytes.NewReader([]byte("cached"))))
	before, err := os.Stat(rw.path(key))
	require.NoError(t, err)
	require.True(t, errors.Is(ro.Add(key, bytes.NewReader(nil)), ErrReadOnly))
	require.True(t, errors.Is(ro.Delete(key), ErrReadOnly))
	r, hit, err := ro.Get(key)
	require.NoError(t, err)
	require.True(t, hit)
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.Equal(t, "cached", string(data))
	after, err := os.Stat(rw.path(key))
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries))
	// An intermediate non-directory gives a deterministic I/O failure even as root.
	_, _, err = NewReadOnly(rw.path(key)).Get(key)
	require.True(t, errors.Is(err, ErrIO))
}
