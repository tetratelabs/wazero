package sys

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestContext_Close(t *testing.T) {
	pathName := "test"
	file := &testFile{}

	fsc := NewFSContext(map[uint32]*FileEntry{
		3: {Path: "."},
		4: {Path: path.Join(".", pathName), File: file},
	})

	// Verify base case
	require.True(t, len(fsc.openedFiles) > 0, "fsc.openedFiles was empty")

	// Closing should not err.
	require.NoError(t, fsc.Close(testCtx))

	// Verify our intended side-effect
	require.Zero(t, len(fsc.openedFiles), "expected no opened files")

	// Verify no error closing again.
	require.NoError(t, fsc.Close(testCtx))
}

func TestContext_Close_Error(t *testing.T) {
	file := &testFile{errors.New("error closing")}

	fsc := NewFSContext(map[uint32]*FileEntry{
		3: {Path: ".", File: file},
		4: {Path: "/", File: file},
	})
	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, len(fsc.openedFiles), "expected no opened files")
}

// compile-time check to ensure testFile implements fs.File
var _ fs.File = &testFile{}

type testFile struct{ closeErr error }

func (f *testFile) Close() error                       { return f.closeErr }
func (f *testFile) Stat() (fs.FileInfo, error)         { return nil, nil }
func (f *testFile) Read(_ []byte) (int, error)         { return 0, nil }
func (f *testFile) Seek(_ int64, _ int) (int64, error) { return 0, nil }
