package sys

import (
	"context"
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestContext_Close(t *testing.T) {
	tempDir := t.TempDir()
	pathName := "test"
	file, _ := createWriteableFile(t, tempDir, pathName, make([]byte, 0))

	fsc := NewFSContext(map[uint32]*FileEntry{
		3: {Path: "."},
		4: {Path: path.Join(".", pathName), File: file},
	})

	// Verify base case
	require.True(t, len(fsc.openedFiles) > 0, "fsc.openedFiles was empty")

	// Closing should not err.
	require.NoError(t, fsc.Close(testCtx))

	// Verify our intended side-effect
	require.Equal(t, 0, len(fsc.openedFiles), "expected no opened files")

	// Verify no error closing again.
	require.NoError(t, fsc.Close(testCtx))
}

// createWriteableFile uses real files when io.Writer tests are needed.
func createWriteableFile(t *testing.T, tmpDir string, pathName string, data []byte) (fs.File, fs.FS) {
	require.NotNil(t, data)
	absolutePath := path.Join(tmpDir, pathName)
	require.NoError(t, os.WriteFile(absolutePath, data, 0o600))

	// open the file for writing in a custom way until #390
	f, err := os.OpenFile(absolutePath, os.O_RDWR, 0o600)
	require.NoError(t, err)
	return f, os.DirFS(tmpDir)
}
