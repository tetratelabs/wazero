package platform

import (
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestOpenFile_Errors(t *testing.T) {
	tmp := t.TempDir()

	t.Run("not found must be ENOENT", func(t *testing.T) {
		_, err := OpenFile(path.Join(tmp, "not-really-exist.txt"), os.O_RDONLY, 0o600)
		require.ErrorIs(t, err, syscall.ENOENT)
	})

	// This is the same as https://github.com/ziglang/zig/blob/d24ebf1d12cf66665b52136a2807f97ff021d78d/lib/std/os/test.zig#L105-L112
	t.Run("try creating on existing file must be EEXIST", func(t *testing.T) {
		filepath := path.Join(tmp, "file.txt")
		f, err := OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		defer require.NoError(t, f.Close())
		require.NoError(t, err)

		_, err = OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		require.ErrorIs(t, err, syscall.EEXIST)
	})
}
