package syscallfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func testOpen_O_RDWR(t *testing.T, tmpDir string, testFS FS) {
	file := "file"
	realPath := path.Join(tmpDir, file)
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

func testOpen_Read(t *testing.T, tmpDir string, testFS FS) {
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
		requireErrno(t, syscall.ENOENT, err)
	})

	t.Run(". opens root", func(t *testing.T) {
		f, err := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		entries := requireReadDir(t, f)
		require.Equal(t, 2, len(entries))
		require.True(t, entries[0].IsDir())
		require.Equal(t, dir, entries[0].Name())
		require.False(t, entries[1].IsDir())
		require.Equal(t, file, entries[1].Name())
	})

	t.Run("dir exists", func(t *testing.T) {
		f, err := testFS.OpenFile(dir, os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		entries := requireReadDir(t, f)
		require.Equal(t, 1, len(entries))
		require.False(t, entries[0].IsDir())
		require.Equal(t, file1, entries[0].Name())
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

	// Make sure O_RDONLY isn't treated bitwise as it is usually zero.
	t.Run("or'd flag", func(t *testing.T) {
		// Example of a flag that can be or'd into O_RDONLY even if not
		// currently supported in WASI or GOOS=js
		const O_NOATIME = 0x40000

		f, err := testFS.OpenFile(file, os.O_RDONLY|O_NOATIME, 0)
		require.NoError(t, err)
		defer f.Close()
	})
}

// requireReadDir ensures the input file is a directory, and returns its
// entries.
func requireReadDir(t *testing.T, f fs.File) []fs.DirEntry {
	if w, ok := f.(io.Writer); ok {
		_, err := w.Write([]byte("hello"))
		requireErrno(t, syscall.EBADF, err)
	}
	// Ensure it implements fs.ReadDirFile
	dir, ok := f.(fs.ReadDirFile)
	require.True(t, ok)
	entries, err := dir.ReadDir(-1)
	require.NoError(t, err)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries
}

func testUtimes(t *testing.T, tmpDir string, testFS FS) {
	file := "file"
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
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

			stat, err := os.Stat(path.Join(tmpDir, tc.path))
			require.NoError(t, err)

			atimeNsec, mtimeNsec, _ := platform.StatTimes(stat)
			if platform.CompilerSupported() {
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}

// testFSAdapter implements fs.FS only to use fstest.TestFS
type testFSAdapter struct {
	fs FS
}

// Open implements the same method as documented on fs.FS
func (f *testFSAdapter) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) { // FS.OpenFile has fewer constraints than fs.FS
		return nil, os.ErrInvalid
	}

	// This isn't a production-grade fs.FS implementation. The only special
	// cases we address here are to pass testfs.TestFS.

	if runtime.GOOS == "windows" {
		switch {
		case strings.Contains(name, "\\"):
			return nil, os.ErrInvalid
		}
	}

	return f.fs.OpenFile(name, os.O_RDONLY, 0)
}

// requireErrno should only be used for functions that wrap the underlying
// syscall.Errno.
func requireErrno(t *testing.T, expected syscall.Errno, actual error) {
	require.True(t, errors.Is(actual, expected), "expected %v, but was %v", expected, actual)
}
