package syscallfs

import (
	"embed"
	_ "embed"
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
	"testing/fstest"
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

var (
	//go:embed testdata/*.txt
	readerAtFS   embed.FS
	readerAtFile = "wazero.txt"
	emptyFile    = "empty.txt"
)

func TestReaderAtOffset(t *testing.T) {
	embedFS, err := fs.Sub(readerAtFS, "testdata")
	require.NoError(t, err)

	d, err := embedFS.Open(readerAtFile)
	require.NoError(t, err)
	defer d.Close()

	bytes, err := io.ReadAll(d)
	require.NoError(t, err)

	mapFS := fstest.MapFS{readerAtFile: &fstest.MapFile{Data: bytes}}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(tmpDir, readerAtFile), bytes, 0o600))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.Open(readerAtFile)
			require.NoError(t, err)
			defer f.Close()

			var r io.Reader = f
			ra := ReaderAtOffset(f, 0)

			requireRead3 := func(r io.Reader, buf []byte) {
				n, err := r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 3, n)
			}

			// The file should work as a reader (base case)
			requireRead3(r, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// The readerAt impl should be able to start from zero also
			requireRead3(ra, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// If the offset didn't change, we expect the next three chars.
			requireRead3(r, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// If state was held between reader-at, we expect the same
			requireRead3(ra, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// We should also be able to make another reader-at
			ra = ReaderAtOffset(f, 3)
			requireRead3(ra, buf)
			require.Equal(t, "ero", string(buf))
		})
	}
}

func TestReaderAtOffset_empty(t *testing.T) {
	embedFS, err := fs.Sub(readerAtFS, "testdata")
	require.NoError(t, err)

	d, err := embedFS.Open(readerAtFile)
	require.NoError(t, err)
	defer d.Close()

	mapFS := fstest.MapFS{emptyFile: &fstest.MapFile{}}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(tmpDir, emptyFile), []byte{}, 0o600))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.Open(emptyFile)
			require.NoError(t, err)
			defer f.Close()

			var r io.Reader = f
			ra := ReaderAtOffset(f, 0)

			requireRead3 := func(r io.Reader, buf []byte) {
				n, err := r.Read(buf)
				require.Equal(t, err, io.EOF)
				require.Equal(t, 0, n) // file is empty
			}

			// The file should work as a reader (base case)
			requireRead3(r, buf)

			// The readerAt impl should be able to start from zero also
			requireRead3(ra, buf)
		})
	}
}

func TestReaderAtOffset_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(readerAtFS, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(emptyFile)
	require.NoError(t, err)
	defer f.Close()

	// mask both io.ReaderAt and io.Seeker
	ra := ReaderAtOffset(struct{ fs.File }{f}, 0)

	buf := make([]byte, 3)
	_, err = ra.Read(buf)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestWriterAtOffset(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)

	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	tests := []struct {
		name string
		fs   FS
	}{
		{name: "syscallfs.dirFS", fs: dirFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.OpenFile(readerAtFile, os.O_RDWR|os.O_CREATE, 0o600)
			require.NoError(t, err)
			defer f.Close()

			w := f.(io.Writer)
			wa := WriterAtOffset(f, 6)

			text := "wazero"
			buf := make([]byte, 3)
			copy(buf, text[:3])

			requireWrite3 := func(r io.Writer, buf []byte) {
				n, err := r.Write(buf)
				require.NoError(t, err)
				require.Equal(t, 3, n)
			}

			// The file should work as a writer (base case)
			requireWrite3(w, buf)

			// The writerAt impl should be able to start from zero also
			requireWrite3(wa, buf)

			copy(buf, text[3:])

			// If the offset didn't change, the next chars will write after the
			// first
			requireWrite3(w, buf)

			// If state was held between writer-at, we expect the same
			requireWrite3(wa, buf)

			// We should also be able to make another writer-at
			wa = WriterAtOffset(f, 12)
			requireWrite3(wa, buf)

			r := ReaderAtOffset(f, 0)
			b, err := io.ReadAll(r)
			require.NoError(t, err)

			// We expect to have written the text two and a half times:
			//  1. io.Write: offset 0
			//  2. io.WriterAt: offset 6
			//  3. second io.WriterAt: offset 12, writing "ero"
			require.Equal(t, text+text+text[3:], string(b))
		})
	}
}

func TestWriterAtOffset_empty(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)

	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	tests := []struct {
		name string
		fs   FS
	}{
		{name: "syscallfs.dirFS", fs: dirFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.OpenFile(emptyFile, os.O_RDWR|os.O_CREATE, 0o600)
			require.NoError(t, err)
			defer f.Close()

			r := f.(io.Writer)
			ra := WriterAtOffset(f, 0)

			var emptyBuf []byte

			requireWrite := func(r io.Writer) {
				n, err := r.Write(emptyBuf)
				require.NoError(t, err)
				require.Equal(t, 0, n) // file is empty
			}

			// The file should work as a writer (base case)
			requireWrite(r)

			// The writerAt impl should be able to start from zero also
			requireWrite(ra)
		})
	}
}

func TestWriterAtOffset_Unsupported(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)

	f, err := dirFS.OpenFile(readerAtFile, os.O_RDWR|os.O_CREATE, 0o600)
	require.NoError(t, err)
	defer f.Close()

	// mask both io.WriterAt and io.Seeker
	ra := WriterAtOffset(struct{ fs.File }{f}, 0)

	buf := make([]byte, 3)
	_, err = ra.Write(buf)
	require.Equal(t, syscall.ENOSYS, err)
}
