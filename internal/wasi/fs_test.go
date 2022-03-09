package internalwasi

import (
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

// TestMemFile_Read_Seek tests the behavior of Read and Seek by iotest.TestReader.
// See iotest.TestReader
func TestMemFile_Read_Seek(t *testing.T) {
	expectedFileContent := []byte("wazero") // arbitrary contents
	memFile := &memFile{
		buf: expectedFileContent,
	}
	// TestReader tests that io.Reader correctly reads the expected contents.
	// It also tests io.Seeker when it's implemented, which memFile does.
	err := iotest.TestReader(memFile, expectedFileContent)
	require.NoError(t, err)
}
