package platform

import (
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSanitizeSeparator(t *testing.T) {
	orig := []byte(filepath.Join("a", "b", "c"))
	SanitizeSeparator(orig)
	require.Equal(t, "a/b/c", string(orig))
}
