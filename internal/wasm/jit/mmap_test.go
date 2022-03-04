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
}

func Test_munmapCodeSegment(t *testing.T) {
	requireSupportedOSArch(t)
	assert := require.New(t)
	newCode, err := mmapCodeSegment(code)
	assert.NoError(err)
	// First munmap should succeed.
	assert.NoError(munmapCodeSegment(newCode))
	// Double munmap should fail.
	assert.Error(munmapCodeSegment(newCode))
}
