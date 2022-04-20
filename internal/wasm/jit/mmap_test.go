package jit

import (
	"crypto/rand"
	"io"
	"testing"

	"github.com/heeus/hwazero/internal/testing/require"
)

var testCode, _ = io.ReadAll(io.LimitReader(rand.Reader, 8*1024))

func Test_mmapCodeSegment(t *testing.T) {
	requireSupportedOSArch(t)
	newCode, err := mmapCodeSegment(testCode)
	require.NoError(t, err)
	// Verify that the mmap is the same as the original.
	require.Equal(t, testCode, newCode)
	// TODO: test newCode can executed.

	t.Run("panic on zero length", func(t *testing.T) {
		captured := require.CapturePanic(func() {
			_, _ = mmapCodeSegment(make([]byte, 0))
		})
		require.EqualError(t, captured, "BUG: mmapCodeSegment with zero length")
	})
}

func Test_munmapCodeSegment(t *testing.T) {
	requireSupportedOSArch(t)

	// Errors if never mapped
	require.Error(t, munmapCodeSegment(testCode))

	newCode, err := mmapCodeSegment(testCode)
	require.NoError(t, err)
	// First munmap should succeed.
	require.NoError(t, munmapCodeSegment(newCode))
	// Double munmap should fail.
	require.Error(t, munmapCodeSegment(newCode))

	t.Run("panic on zero length", func(t *testing.T) {
		captured := require.CapturePanic(func() {
			_ = munmapCodeSegment(make([]byte, 0))
		})
		require.EqualError(t, captured, "BUG: munmapCodeSegment with zero length")
	})
}
