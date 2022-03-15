package jit

import (
	"crypto/rand"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

var code, _ = io.ReadAll(io.LimitReader(rand.Reader, 8*1024))

func Test_mmapCodeSegment(t *testing.T) {
	requireSupportedOSArch(t)
	assert := require.New(t)
	newCode, err := mmapCodeSegment(code)
	assert.NoError(err)
	// Verify that the mmap is the same as the original.
	assert.Equal(code, newCode)
	// TODO: test newCode can executed.

	t.Run("panic on zero length", func(t *testing.T) {
		require.PanicsWithError(t, "BUG: mmapCodeSegment with zero length", func() {
			_, _ = mmapCodeSegment(make([]byte, 0))
		})
	})
}

func Test_munmapCodeSegment(t *testing.T) {
	requireSupportedOSArch(t)
	assert := require.New(t)

	// Errors if never mapped
	assert.Error(munmapCodeSegment(code))

	newCode, err := mmapCodeSegment(code)
	assert.NoError(err)
	// First munmap should succeed.
	assert.NoError(munmapCodeSegment(newCode))
	// Double munmap should fail.
	assert.Error(munmapCodeSegment(newCode))

	t.Run("panic on zero length", func(t *testing.T) {
		require.PanicsWithError(t, "BUG: munmapCodeSegment with zero length", func() {
			_ = munmapCodeSegment(make([]byte, 0))
		})
	})
}
