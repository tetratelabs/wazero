package custom

import (
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_ToJsMode(t *testing.T) {
	t.Run("/dev/null", func(t *testing.T) {
		st, err := os.Stat(os.DevNull)
		require.NoError(t, err)

		fm := ToJsMode(st.Mode())

		// Should be a character device, and retain the permissions.
		require.Equal(t, S_IFCHR|uint32(st.Mode().Perm()), fm)
	})
}

func Test_FromJsMode(t *testing.T) {
	t.Run("sticky bit", func(t *testing.T) {
		jsMode := ToJsMode(0o0755 | fs.ModeSticky)
		require.Equal(t, 0o0755|S_IFREG|S_ISVTX, jsMode)
	})
}
