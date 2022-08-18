package platform

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var testCodeBuf, _ = io.ReadAll(io.LimitReader(rand.Reader, 8*1024))

func Test_MmapCodeSegment(t *testing.T) {
	if !CompilerSupported() {
		t.Skip()
	}

	testCodeReader := bytes.NewReader(testCodeBuf)
	newCode, err := MmapCodeSegment(testCodeReader, testCodeReader.Len())
	require.NoError(t, err)
	// Verify that the mmap is the same as the original.
	require.Equal(t, testCodeBuf, newCode)
	// TODO: test newCode can executed.

	t.Run("panic on zero length", func(t *testing.T) {
		captured := require.CapturePanic(func() {
			_, _ = MmapCodeSegment(bytes.NewBuffer(make([]byte, 0)), 0)
		})
		require.EqualError(t, captured, "BUG: MmapCodeSegment with zero length")
	})
}

func Test_MunmapCodeSegment(t *testing.T) {
	if !CompilerSupported() {
		t.Skip()
	}

	// Errors if never mapped
	require.Error(t, MunmapCodeSegment(testCodeBuf))

	testCodeReader := bytes.NewReader(testCodeBuf)
	newCode, err := MmapCodeSegment(testCodeReader, testCodeReader.Len())
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
