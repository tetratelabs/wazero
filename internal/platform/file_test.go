package platform

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	gofstest "testing/fstest"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var _ File = NoopFile{}

// NoopFile shows the minimal methods a type embedding UnimplementedFile must
// implement.
type NoopFile struct {
	UnimplementedFile
}

// The current design requires the user to implement Path.
func (NoopFile) Path() string {
	return ""
}

// The current design requires the user to implement AccessMode.
func (NoopFile) AccessMode() int {
	return syscall.O_RDONLY
}

// The current design requires the user to consciously implement Close.
// However, we could change UnimplementedFile to return zero.
func (NoopFile) Close() (errno syscall.Errno) { return }

// Once File.File is removed, it will be possible to implement NoopFile.
func (NoopFile) File() fs.File { panic("noop") }

//go:embed file_test.go
var embedFS embed.FS

var (
	//go:embed testdata
	testdata  embed.FS
	preadFile = "wazero.txt"
	emptyFile = "empty.txt"
)

func TestFsFileReadAndPread(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(preadFile)
	require.NoError(t, err)
	defer f.Close()

	bytes, err := io.ReadAll(f)
	require.NoError(t, err)

	mapFS := gofstest.MapFS{preadFile: &gofstest.MapFile{Data: bytes}}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(tmpDir, preadFile), bytes, 0o600))
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
			f, err := tc.fs.Open(preadFile)
			require.NoError(t, err)
			defer f.Close()

			fs := NewFsFile(preadFile, syscall.O_RDONLY, f)

			// The file should be readable (base case)
			requireRead(t, fs, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// We should be able to pread from zero also
			requirePread(t, fs, buf, 0)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// If the offset didn't change, read should expect the next three chars.
			requireRead(t, fs, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// We should also be able pread from any offset
			requirePread(t, fs, buf, 2)
			require.Equal(t, "zer", string(buf))
		})
	}
}

func requireRead(t *testing.T, f File, buf []byte) {
	n, errno := f.Read(buf)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func requirePread(t *testing.T, f File, buf []byte, off int64) {
	n, errno := f.Pread(buf, off)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func TestFsFileRead_empty(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(preadFile)
	require.NoError(t, err)
	defer f.Close()

	mapFS := gofstest.MapFS{emptyFile: &gofstest.MapFile{}}

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

			fs := NewFsFile(preadFile, syscall.O_RDONLY, f)

			t.Run("Read", func(t *testing.T) {
				// We should be able to read an empty file
				n, errno := fs.Read(buf)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, n)
			})

			t.Run("Pread", func(t *testing.T) {
				n, errno := fs.Pread(buf, 0)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, n)
			})
		})
	}
}

func TestFsFilePread_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(emptyFile)
	require.NoError(t, err)
	defer f.Close()

	// mask both io.ReaderAt and io.Seeker
	f = struct{ fs.File }{f}

	fs := NewFsFile(preadFile, syscall.O_RDONLY, f)

	buf := make([]byte, 3)
	_, errno := fs.Pread(buf, 0)
	require.EqualErrno(t, syscall.ENOSYS, errno)
}

func TestFsFileWriteAndPwrite(t *testing.T) {
	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	path := path.Join(t.TempDir(), preadFile)
	f := openFsFile(t, path, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	text := "wazero"
	buf := make([]byte, 3)
	copy(buf, text[:3])

	// The file should be writeable
	requireWrite(t, f, buf)

	// We should be able to pwrite at gap
	requirePwrite(t, f, buf, 6)

	copy(buf, text[3:])

	// If the offset didn't change, the next chars will write after the
	// first
	requireWrite(t, f, buf)

	// We should be able to pwrite the same bytes as above
	requirePwrite(t, f, buf, 9)

	// We should also be able to pwrite past the above.
	requirePwrite(t, f, buf, 12)

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	// We expect to have written the text two and a half times:
	//  1. Write: (file offset 0) "waz"
	//  2. Pwrite: offset 6 "waz"
	//  3. Write: (file offset 3) "ero"
	//  4. Pwrite: offset 9 "ero"
	//  4. Pwrite: offset 12 "ero"
	require.Equal(t, "wazerowazeroero", string(b))
}

func requireWrite(t *testing.T, f File, buf []byte) {
	n, errno := f.Write(buf)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func requirePwrite(t *testing.T, f File, buf []byte, off int64) {
	n, errno := f.Pwrite(buf, off)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func TestFsFileWrite_empty(t *testing.T) {
	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	path := path.Join(t.TempDir(), emptyFile)
	f := openFsFile(t, path, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	tests := []struct {
		name string
		fn   func(File, []byte) (int, syscall.Errno)
	}{
		{name: "Write", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Write(buf)
		}},
		{name: "Pwrite from zero", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 0)
		}},
		{name: "Pwrite from 3", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 3)
		}},
	}

	var emptyBuf []byte

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			n, errno := tc.fn(f, emptyBuf)
			require.EqualErrno(t, 0, errno)
			require.Zero(t, n)

			// The file should be empty
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Zero(t, len(b))
		})
	}
}

func TestFsFileWrite_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(preadFile)
	require.NoError(t, err)
	defer f.Close()

	tests := []struct {
		name string
		fn   func(File, []byte) (int, syscall.Errno)
	}{
		{name: "Write", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Write(buf)
		}},
		{name: "Pwrite", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 0)
		}},
	}

	buf := []byte("wazero")

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			// Use syscall.O_RDWR so that it fails due to type not flags
			f := NewFsFile(preadFile, syscall.O_RDWR, f)
			_, errno := tc.fn(f, buf)
			require.EqualErrno(t, syscall.ENOSYS, errno)
		})
	}
}

func TestFsFileWrite_BadFile(t *testing.T) {
	// Create the file
	path := path.Join(t.TempDir(), emptyFile)
	of, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, of.Close())

	// Open the file read-only
	flag := syscall.O_RDONLY
	f := openFsFile(t, path, flag, 0o600)
	defer f.Close()

	tests := []struct {
		name string
		fn   func(File, []byte) (int, syscall.Errno)
	}{
		{name: "Write", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Write(buf)
		}},
		{name: "Pwrite", fn: func(f File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 0)
		}},
	}

	buf := []byte("wazero")

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			_, errno := tc.fn(f, buf)

			// The descriptor exists, but not open for writing
			require.EqualErrno(t, syscall.EBADF, errno)
		})
	}
}

func TestFsFileSync_NoError(t *testing.T) {
	testSync_NoError(t, File.Sync)
}

func TestFsFileDatasync_NoError(t *testing.T) {
	testSync_NoError(t, File.Datasync)
}

func testSync_NoError(t *testing.T, sync func(File) syscall.Errno) {
	roPath := "file_test.go"
	ro, err := embedFS.Open(roPath)
	require.NoError(t, err)
	defer ro.Close()

	rwPath := path.Join(t.TempDir(), "datasync")
	rw, err := os.Create(rwPath)
	require.NoError(t, err)
	defer rw.Close()

	tests := []struct {
		name string
		f    File
	}{
		{
			name: "UnimplementedFile",
			f:    NoopFile{},
		},
		{
			name: "File of read-only fs.File",
			f:    NewFsFile(roPath, syscall.O_RDONLY, ro),
		},
		{
			name: "File of os.File",
			f:    NewFsFile(rwPath, syscall.O_RDWR, rw),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			require.Zero(t, sync(tc.f))
		})
	}
}

func TestFsFileSync(t *testing.T) {
	testSync(t, File.Sync)
}

func TestFsFileDatasync(t *testing.T) {
	testSync(t, File.Datasync)
}

// testSync doesn't guarantee sync works because the operating system may
// sync anyway. There is no test in Go for syscall.Fdatasync, but closest is
// similar to below. Effectively, this only tests that things don't error.
func testSync(t *testing.T, sync func(File) syscall.Errno) {
	dPath := t.TempDir()
	d, err := os.Open(dPath)
	require.NoError(t, err)
	defer d.Close()

	// Even though it is invalid, try to sync a directory
	errno := sync(NewFsFile(dPath, syscall.O_RDONLY, d))
	require.EqualErrno(t, 0, errno)

	fPath := path.Join(dPath, t.Name())

	f := openFsFile(t, fPath, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, errno = f.Write([]byte(expected))
	require.EqualErrno(t, 0, errno)

	// Sync the data.
	errno = sync(f)
	require.EqualErrno(t, 0, errno)

	// Rewind while the file is still open.
	_, err = f.File().(io.Seeker).Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	buf := make([]byte, 50)
	n, errno := f.Read(buf)
	require.EqualErrno(t, 0, errno)

	// It may be the case that sync worked.
	require.Equal(t, expected, string(buf[:n]))

	// Windows allows you to sync a closed file
	if runtime.GOOS != "windows" {
		testEBADFIfFileClosed(t, sync)
	}
}

func TestFsFileTruncate(t *testing.T) {
	content := []byte("123456")

	tests := []struct {
		name            string
		size            int64
		expectedContent []byte
		expectedErr     error
	}{
		{
			name:            "one less",
			size:            5,
			expectedContent: []byte("12345"),
		},
		{
			name:            "same",
			size:            6,
			expectedContent: content,
		},
		{
			name:            "zero",
			size:            0,
			expectedContent: []byte(""),
		},
		{
			name:            "larger",
			size:            106,
			expectedContent: append(content, make([]byte, 100)...),
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			f := openForWrite(t, path.Join(tmpDir, tc.name), content)
			defer f.Close()

			errno := f.Truncate(tc.size)
			require.EqualErrno(t, 0, errno)

			actual, err := os.ReadFile(f.Path())
			require.NoError(t, err)
			require.Equal(t, tc.expectedContent, actual)
		})
	}

	truncateToZero := func(f File) syscall.Errno {
		return f.Truncate(0)
	}

	if runtime.GOOS != "windows" {
		// TODO: os.Truncate on windows can create the file even when it
		// doesn't exist.
		testEBADFIfFileClosed(t, truncateToZero)
	}

	testEISDIR(t, truncateToZero)

	t.Run("negative", func(t *testing.T) {
		tmpDir := t.TempDir()

		f := openForWrite(t, path.Join(tmpDir, "truncate"), content)
		defer f.Close()

		errno := f.Truncate(-1)
		require.EqualErrno(t, syscall.EINVAL, errno)
	})
}

func testEBADFIfFileClosed(t *testing.T, fn func(File) syscall.Errno) bool {
	return t.Run("EBADF if file closed", func(t *testing.T) {
		tmpDir := t.TempDir()

		f := openForWrite(t, path.Join(tmpDir, "EBADF"), []byte{1, 2, 3, 4})

		// close the file underneath
		require.Zero(t, f.Close())

		require.EqualErrno(t, syscall.EBADF, fn(f))
	})
}

func testEISDIR(t *testing.T, fn func(File) syscall.Errno) bool {
	return t.Run("EISDIR if directory", func(t *testing.T) {
		f := openFsFile(t, os.TempDir(), syscall.O_RDONLY|O_DIRECTORY, 0o666)
		defer f.Close()

		require.EqualErrno(t, syscall.EISDIR, fn(f))
	})
}

func openForWrite(t *testing.T, path string, content []byte) File {
	require.NoError(t, os.WriteFile(path, content, 0o0600))
	return openFsFile(t, path, syscall.O_RDWR, 0o666)
}

func openFsFile(t *testing.T, path string, flag int, perm fs.FileMode) File {
	f, errno := OpenFile(path, flag, perm)
	require.EqualErrno(t, 0, errno)
	return NewFsFile(path, flag, f)
}
