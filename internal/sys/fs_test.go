package sys

import (
	"context"
	"errors"
	"testing"

	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestEmptyFS(t *testing.T) {
	testFS := EmptyFS

	t.Run("validates path", func(t *testing.T) {
		f, err := testFS.Open("/foo.txt")
		require.Nil(t, f)
		require.EqualError(t, err, "open /foo.txt: invalid argument")
	})

	t.Run("path not found", func(t *testing.T) {
		f, err := testFS.Open("foo.txt")
		require.Nil(t, f)
		require.EqualError(t, err, "open foo.txt: file does not exist")
	})
}

func TestEmptyFSContext(t *testing.T) {
	testFS := emptyFSContext
	expected := &FSContext{
		fs:          EmptyFS,
		openedFiles: map[uint32]*FileEntry{},
		lastFD:      2,
	}

	t.Run("OpenFile doesn't affect state", func(t *testing.T) {
		fd, err := testFS.OpenFile(testCtx, "foo.txt")
		require.Zero(t, fd)
		require.EqualError(t, err, "open foo.txt: file does not exist")

		// Ensure this didn't modify state
		require.Equal(t, expected, testFS)
	})

	t.Run("Close doesn't affect state", func(t *testing.T) {
		err := testFS.Close(testCtx)
		require.NoError(t, err)

		// Ensure this didn't modify state
		require.Equal(t, expected, testFS)
	})
}

func TestContext_Close(t *testing.T) {
	fsc := NewFSContext(testfs.FS{"foo": &testfs.File{}})
	_, err := fsc.OpenFile(testCtx, "/foo")
	require.NoError(t, err)

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
	file := &testfs.File{CloseErr: errors.New("error closing")}
	fsc := NewFSContext(testfs.FS{"foo": file})
	_, err := fsc.OpenFile(testCtx, "/foo")
	require.NoError(t, err)

	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, len(fsc.openedFiles), "expected no opened files")
}
