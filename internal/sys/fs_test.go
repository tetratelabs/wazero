package sys

import (
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
	"testing/fstest"

	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

var (
	noopStdin  = &FileEntry{Name: noopStdinStat.Name(), File: &stdioFileReader{r: eofReader{}, s: noopStdinStat}}
	noopStdout = &FileEntry{Name: noopStdoutStat.Name(), File: &stdioFileWriter{w: io.Discard, s: noopStdoutStat}}
	noopStderr = &FileEntry{Name: noopStderrStat.Name(), File: &stdioFileWriter{w: io.Discard, s: noopStderrStat}}
)

//go:embed testdata
var testdata embed.FS

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
			name: "os.DirFS",
			// Don't use "testdata" because it may not be present in
			// cross-architecture (a.k.a. scratch) build containers.
			fs:           os.DirFS("."),
			expectOsFile: true,
		},
		{
			name: "fstest.MapFS",
			fs:   fstest.MapFS{},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			fsc, err := NewFSContext(nil, nil, nil, tc.fs)
			require.NoError(t, err)
			defer fsc.Close(testCtx)

			require.Equal(t, tc.fs, fsc.fs)
			require.Equal(t, "/", fsc.openedFiles[FdRoot].Name)
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
	testFS, err := NewFSContext(nil, nil, nil, EmptyFS)
	require.NoError(t, err)

	expected := &FSContext{
		fs: EmptyFS,
		openedFiles: map[uint32]*FileEntry{
			FdStdin:  noopStdin,
			FdStdout: noopStdout,
			FdStderr: noopStderr,
		},
		lastFD: FdStderr,
	}

	t.Run("OpenFile doesn't affect state", func(t *testing.T) {
		fd, err := testFS.OpenFile("foo.txt")
		require.Zero(t, fd)
		require.EqualError(t, err, "open foo.txt: file does not exist")

		// Ensure this didn't modify state
		require.Equal(t, expected, testFS)
	})

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close(testCtx)
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{
			fs:          EmptyFS,
			openedFiles: map[uint32]*FileEntry{},
			lastFD:      FdStderr,
		}, testFS)
	})
}

func TestContext_File(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	fsc, err := NewFSContext(nil, nil, nil, embedFS)
	require.NoError(t, err)
	defer fsc.Close(testCtx)

	tests := []struct {
		name     string
		expected string
	}{
		{
			name: "empty.txt",
		},
		{
			name:     "test.txt",
			expected: "animals\n",
		},
		{
			name:     "sub/test.txt",
			expected: "greet sub dir\n",
		},
		{
			name:     "sub/sub/test.txt",
			expected: "greet sub sub dir\n",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			fd, err := fsc.OpenFile(tc.name)
			require.NoError(t, err)
			defer fsc.CloseFile(fd)

			f, ok := fsc.OpenedFile(fd)
			require.True(t, ok)

			stat, err := f.File.Stat()
			require.NoError(t, err)

			// Ensure the name is the basename and matches the stat name.
			require.Equal(t, path.Base(tc.name), f.Name)
			require.Equal(t, f.Name, stat.Name())

			buf := make([]byte, stat.Size())
			size, err := f.File.Read(buf)
			if err != nil {
				require.Equal(t, io.EOF, err)
			}
			require.Equal(t, stat.Size(), int64(size))

			require.Equal(t, tc.expected, string(buf[:size]))
		})
	}
}

func TestContext_Close(t *testing.T) {
	fsc, err := NewFSContext(nil, nil, nil, testfs.FS{"foo": &testfs.File{}})
	require.NoError(t, err)
	// Verify base case
	require.Equal(t, 1+FdRoot, uint32(len(fsc.openedFiles)))

	_, err = fsc.OpenFile("foo")
	require.NoError(t, err)
	require.Equal(t, 2+FdRoot, uint32(len(fsc.openedFiles)))

	// Closing should not err.
	require.NoError(t, fsc.Close(testCtx))

	// Verify our intended side-effect
	require.Zero(t, len(fsc.openedFiles))

	// Verify no error closing again.
	require.NoError(t, fsc.Close(testCtx))
}

func TestContext_Close_Error(t *testing.T) {
	file := &testfs.File{CloseErr: errors.New("error closing")}
	fsc, err := NewFSContext(nil, nil, nil, testfs.FS{"foo": file})
	require.NoError(t, err)

	// open another file
	_, err = fsc.OpenFile("foo")
	require.NoError(t, err)

	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, len(fsc.openedFiles), "expected no opened files")
}
