package platform

import (
	"embed"
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

func TestFileSync(t *testing.T) {
	ro, err := embedFS.Open("file_test.go")
	require.NoError(t, err)
	defer ro.Close()

	rw, err := os.Create(path.Join(t.TempDir(), "sync"))
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
			require.Zero(t, tc.f.Sync())
		})
	}
}
