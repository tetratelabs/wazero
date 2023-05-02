package platform

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
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
	ro, err := embedFS.Open("file_test.go")
	require.NoError(t, err)
	defer ro.Close()

	rw, err := os.Create(path.Join(t.TempDir(), "datasync"))
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
			f:    &DefaultFile{F: ro},
		},
		{
			name: "File of os.File",
			f:    &DefaultFile{F: rw},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			require.Zero(t, sync(tc.f))
		})
	}
}

func TestFileSync(t *testing.T) {
	testSync(t, File.Sync)
}

func TestFileDatasync(t *testing.T) {
	testSync(t, File.Datasync)
}

// testSync doesn't guarantee sync works because the operating system may
// sync anyway. There is no test in Go for syscall.Fdatasync, but closest is
// similar to below. Effectively, this only tests that things don't error.
func testSync(t *testing.T, sync func(File) syscall.Errno) {
	f, errno := os.CreateTemp("", t.Name())
	require.NoError(t, errno)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, errno = f.Write([]byte(expected))
	require.NoError(t, errno)

	// Sync the data.
	if errno = sync(&DefaultFile{F: f}); errno == syscall.ENOSYS {
		return // don't continue if it isn't supported.
	}
	require.Zero(t, errno)

	// Rewind while the file is still open.
	_, err := f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	var buf bytes.Buffer
	_, errno = io.Copy(&buf, f)
	require.NoError(t, errno)

	// It may be the case that sync worked.
	require.Equal(t, expected, buf.String())
}
