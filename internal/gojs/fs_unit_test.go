package gojs

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_getWasiFiletype_DevNull(t *testing.T) {
	st, err := os.Stat(os.DevNull)
	require.NoError(t, err)

	fm := getJsMode(st.Mode())

	// Should be a character device, and retain the permissions.
	require.Equal(t, S_IFCHR|uint32(st.Mode().Perm()), fm)
}
