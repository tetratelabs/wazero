package syscallfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func testOpen_O_RDWR(t *testing.T, rootDir string, testFS FS) {
	file := "file"
	realPath := path.Join(rootDir, file)
	err := os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	f, err := testFS.OpenFile(file, os.O_RDWR, 0)
	require.NoError(t, err)
	defer f.Close()

	w, ok := f.(io.Writer)
	require.True(t, ok)

	// If the write flag was honored, we should be able to write!
	fileContents := []byte{1, 2, 3, 4}
	n, err := w.Write(fileContents)
	require.NoError(t, err)
	require.Equal(t, len(fileContents), n)

	// Verify the contents actually wrote.
	b, err := os.ReadFile(realPath)
	require.NoError(t, err)
	require.Equal(t, fileContents, b)
}

func testOpen_Read(t *testing.T, rootDir string, testFS FS) {
	file := "file"
	fileContents := []byte{1, 2, 3, 4}
	err := os.WriteFile(path.Join(rootDir, file), fileContents, 0o700)
	require.NoError(t, err)

	dir := "dir"
	dirRealPath := path.Join(rootDir, dir)
	err = os.Mkdir(dirRealPath, 0o700)
	require.NoError(t, err)

	file1 := "file1"
	fileInDir := path.Join(dirRealPath, file1)
	require.NoError(t, os.WriteFile(fileInDir, []byte{2}, 0o600))

	t.Run("doesn't exist", func(t *testing.T) {
		_, err := testFS.OpenFile("nope", os.O_RDONLY, 0)

		// We currently follow os.Open not syscall.Open, so the error is wrapped.
		requireErrno(t, syscall.ENOENT, err)
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

		if w, ok := f.(io.Writer); ok {
			_, err := w.Write([]byte("hello"))
			requireErrno(t, syscall.EBADF, err)
		}
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

		if w, ok := f.(io.Writer); ok {
			_, err := w.Write([]byte("hello"))
			if runtime.GOOS == "windows" {
				requireErrno(t, syscall.EPERM, err)
			} else {
				requireErrno(t, syscall.EBADF, err)
			}
		}
	})
}

func testUtimes(t *testing.T, rootDir string, testFS FS) {
	file := "file"
	err := os.WriteFile(path.Join(rootDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(rootDir, dir), 0o700)
	require.NoError(t, err)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Utimes("nope",
			time.Unix(123, 4*1e3).UnixNano(),
			time.Unix(567, 8*1e3).UnixNano())
		require.Equal(t, syscall.ENOENT, err)
	})

	type test struct {
		name                 string
		path                 string
		atimeNsec, mtimeNsec int64
	}

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{
			name:      "file positive",
			path:      file,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{
			name:      "dir positive",
			path:      dir,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "file zero", path: file},
		{name: "dir zero", path: dir},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{
				name:      "file negative",
				path:      file,
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
			test{
				name:      "dir negative",
				path:      dir,
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := testFS.Utimes(tc.path, tc.atimeNsec, tc.mtimeNsec)
			require.NoError(t, err)

			stat, err := os.Stat(path.Join(rootDir, tc.path))
			require.NoError(t, err)

			atimeNsec, mtimeNsec, _ := platform.StatTimes(stat)
			if platform.CompilerSupported() {
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}

// requireErrno should only be used for functions that wrap the underlying
// syscall.Errno.
func requireErrno(t *testing.T, expected syscall.Errno, actual error) {
	require.True(t, errors.Is(actual, expected), "expected %v, but was %v", expected, actual)
}
