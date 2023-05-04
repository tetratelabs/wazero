package platform

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"

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
	return "noop"
}

// The current design requires the user to consciously implement Close.
// However, we could change UnimplementedFile to return zero.
func (NoopFile) Close() (errno syscall.Errno) { return }

// Once File.File is removed, it will be possible to implement NoopFile.
func (NoopFile) File() fs.File { panic("noop") }

//go:embed file_test.go
var embedFS embed.FS

func TestFileSync_NoError(t *testing.T) {
	testSync_NoError(t, File.Sync)
}

func TestFileDatasync_NoError(t *testing.T) {
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
			f:    NewFsFile(roPath, ro),
		},
		{
			name: "File of os.File",
			f:    NewFsFile(rwPath, rw),
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
	errno := sync(NewFsFile(dPath, d))
	require.EqualErrno(t, 0, errno)

	fPath := path.Join(dPath, t.Name())

	f := openFsFile(t, fPath, os.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, err = f.File().(io.Writer).Write([]byte(expected))
	require.NoError(t, err)

	// Sync the data.
	errno = sync(f)
	require.EqualErrno(t, 0, errno)

	// Rewind while the file is still open.
	_, err = f.File().(io.Seeker).Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	var buf bytes.Buffer
	_, err = io.Copy(&buf, f.File().(io.Reader))
	require.NoError(t, err)

	// It may be the case that sync worked.
	require.Equal(t, expected, buf.String())

	// Windows allows you to sync a closed file
	if runtime.GOOS != "windows" {
		testEBADFIfFileClosed(t, sync)
	}
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

func openForWrite(t *testing.T, path string, content []byte) File {
	require.NoError(t, os.WriteFile(path, content, 0o0600))
	return openFsFile(t, path, os.O_RDWR, 0o666)
}

func openFsFile(t *testing.T, path string, flag int, perm fs.FileMode) File {
	f, errno := OpenFile(path, flag, perm)
	require.EqualErrno(t, 0, errno)
	return NewFsFile(path, f)
}
