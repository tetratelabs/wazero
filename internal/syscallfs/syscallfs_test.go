package syscallfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	pathutil "path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestDirFS_Open_Read(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = pathutil.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))

	testFS := Adapt(dirFS(tmpDir))

	testFS_Open_Read(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// syscall.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func testFS_Open_Read(t *testing.T, tmpDir string, testFS FS) {
	file := "file"
	fileContents := []byte{1, 2, 3, 4}
	err := os.WriteFile(path.Join(tmpDir, file), fileContents, 0o700)
	require.NoError(t, err)

	dir := "dir"
	dirRealPath := path.Join(tmpDir, dir)
	err = os.Mkdir(dirRealPath, 0o700)
	require.NoError(t, err)

	file1 := "file1"
	fileInDir := path.Join(dirRealPath, file1)
	require.NoError(t, os.WriteFile(fileInDir, []byte{2}, 0o600))

	t.Run("doesn't exist", func(t *testing.T) {
		_, err := testFS.OpenFile("nope", os.O_RDONLY, 0)

		// We currently follow os.Open not syscall.Open, so the error is wrapped.
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))
	})

	t.Run("dir exists", func(t *testing.T) {
		f, err := testFS.OpenFile(dir, os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		// Ensure it implements fs.ReadDirFile
		d, ok := f.(fs.ReadDirFile)
		require.True(t, ok)
		e, err := d.ReadDir(-1)
		require.NoError(t, err)
		require.Equal(t, 1, len(e))
		require.False(t, e[0].IsDir())
		require.Equal(t, file1, e[0].Name())

		// Ensure it doesn't implement io.Writer
		_, ok = f.(io.Writer)
		require.False(t, ok)
	})

	t.Run("file exists", func(t *testing.T) {
		f, err := testFS.OpenFile(file, os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		// Ensure it implements io.ReaderAt
		r, ok := f.(io.ReaderAt)
		require.True(t, ok)
		lenToRead := len(fileContents) - 1
		buf := make([]byte, lenToRead)
		n, err := r.ReadAt(buf, 1)
		require.NoError(t, err)
		require.Equal(t, lenToRead, n)
		require.Equal(t, fileContents[1:], buf)

		// Ensure it implements io.Seeker
		s, ok := f.(io.Seeker)
		require.True(t, ok)
		offset, err := s.Seek(1, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(1), offset)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		require.Equal(t, fileContents[1:], b)

		// Ensure it doesn't implement io.Writer
		_, ok = f.(io.Writer)
		require.False(t, ok)
	})
}
