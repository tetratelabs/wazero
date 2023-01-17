package syscallfs

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestEmptyFS_String(t *testing.T) {
	require.Equal(t, "empty:/:ro", EmptyFS.String())
}
