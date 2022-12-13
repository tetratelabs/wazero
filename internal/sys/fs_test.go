package sys

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

//go:embed testdata
var testdata embed.FS

var testFS = fstest.MapFS{
	"empty.txt":    {},
	"test.txt":     {Data: []byte("animals\n")},
	"sub":          {Mode: fs.ModeDir},
	"sub/test.txt": {Data: []byte("greet sub dir\n")},
}

func TestNewFSContext(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	// Test various usual configuration for the file system.
	tests := []struct {
		name         string
		fs           fs.FS
		expectOsFile bool
	}{
		{
			name: "embed.FS",
			fs:   embedFS,
		},
		{
			name:         "os.DirFS",
			fs:           os.DirFS("testdata"),
			expectOsFile: true,
		},
		{
			name: "fstest.MapFS",
			fs:   testFS,
		},
		{
			name: "fstest.MapFS",
			fs:   testFS,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			fsc, err := NewFSContext(tc.fs)
			require.NoError(t, err)
			defer fsc.Close(testCtx)

			require.Equal(t, tc.fs, fsc.fs)
			require.Equal(t, "/", fsc.openedFiles[FdRoot].Path)
			rootFile := fsc.openedFiles[FdRoot].File
			require.NotNil(t, rootFile)

			_, osFile := rootFile.(*os.File)
			require.Equal(t, tc.expectOsFile, osFile)
		})
	}
}

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
	fsc, err := NewFSContext(testfs.FS{"foo": &testfs.File{}})
	require.NoError(t, err)
	// Verify base case
	require.Equal(t, 1, len(fsc.openedFiles))

	_, err = fsc.OpenFile(testCtx, "foo")
	require.NoError(t, err)
	require.Equal(t, 2, len(fsc.openedFiles))

	// Closing should not err.
	require.NoError(t, fsc.Close(testCtx))

	// Verify our intended side-effect
	require.Zero(t, len(fsc.openedFiles))

	// Verify no error closing again.
	require.NoError(t, fsc.Close(testCtx))
}

func TestContext_Close_Error(t *testing.T) {
	file := &testfs.File{CloseErr: errors.New("error closing")}
	fsc, err := NewFSContext(testfs.FS{"foo": file})
	require.NoError(t, err)

	// open another file
	_, err = fsc.OpenFile(testCtx, "foo")
	require.NoError(t, err)

	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, len(fsc.openedFiles), "expected no opened files")
}
