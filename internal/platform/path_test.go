package platform

import (
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestToPosixPath(t *testing.T) {
	orig := filepath.Join("a", "b", "c")
	fixed := ToPosixPath(orig)
	require.Equal(t, "a/b/c", fixed)
}
