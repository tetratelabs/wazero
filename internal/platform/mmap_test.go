package platform

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_MmapCodeSegment(t *testing.T) {
	if !CompilerSupported() {
		t.Skip()
	}

	_, err := MmapCodeSegment(1234)
	require.NoError(t, err)
	t.Run("panic on zero length", func(t *testing.T) {
		captured := require.CapturePanic(func() {
			_, _ = MmapCodeSegment(0)
		})
		require.EqualError(t, captured, "BUG: MmapCodeSegment with zero length")
	})
}

func Test_MunmapCodeSegment(t *testing.T) {
	if !CompilerSupported() {
		t.Skip()
	}

	// Errors if never mapped
	require.Error(t, MunmapCodeSegment([]byte{1, 2, 3, 5}))

	newCode, err := MmapCodeSegment(100)
	require.NoError(t, err)
	// First munmap should succeed.
	require.NoError(t, MunmapCodeSegment(newCode))
	// Double munmap should fail.
	require.Error(t, MunmapCodeSegment(newCode))

	t.Run("panic on zero length", func(t *testing.T) {
		captured := require.CapturePanic(func() {
			_ = MunmapCodeSegment(make([]byte, 0))
		})
		require.EqualError(t, captured, "BUG: MunmapCodeSegment with zero length")
	})
}
