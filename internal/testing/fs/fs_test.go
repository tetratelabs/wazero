package testfs

import (
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFS(t *testing.T) {
	testFS := &FS{}

	t.Run("path not found", func(t *testing.T) {
		f, err := testFS.Open("foo.txt")
		require.Nil(t, f)
		require.EqualError(t, err, "open foo.txt: file does not exist")
	})

	(*testFS)["foo.txt"] = &File{}
	f, err := testFS.Open("foo.txt")
	require.NoError(t, err)
	require.Equal(t, f, &File{})
}

func TestFile(t *testing.T) {
	f := &File{CloseErr: fs.ErrClosed}

	t.Run("returns close error", func(t *testing.T) {
		err := f.Close()
		require.Equal(t, fs.ErrClosed, err)
	})
}
