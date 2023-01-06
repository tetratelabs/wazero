package syscallfs_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/syscallfs"
	"github.com/tetratelabs/wazero/internal/syscallfs/syscallfstest"
)

func TestDirFS(t *testing.T) {
	syscallfstest.TestReadWriteFS(t, func(t *testing.T) syscallfs.FS {
		fsys, err := syscallfs.NewDirFS(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return fsys
	})
}
