package wasm

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestDefaultSysContext(t *testing.T) {
	sys, err := NewSysContext(
		0,   // max
		nil, // args
		nil, // environ
		nil, // stdin
		nil, // stdout
		nil, // stderr
		nil, // openedFiles
	)
	require.NoError(t, err)

	require.Nil(t, sys.Args())
	require.Zero(t, sys.ArgsSize())
	require.Nil(t, sys.Environ())
	require.Zero(t, sys.EnvironSize())
	require.Equal(t, eofReader{}, sys.Stdin())
	require.Equal(t, io.Discard, sys.Stdout())
	require.Equal(t, io.Discard, sys.Stderr())
	require.Empty(t, sys.openedFiles)

	require.Equal(t, sys, DefaultSysContext())
}

func TestNewSysContext_Args(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			args:         []string{"a", "bc"},
			expectedSize: 5,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			args:        []string{"a", "bc"},
			expectedErr: "args invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			args:        []string{"a", "bc"},
			expectedErr: "args invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			args:        []string{"a", string([]byte{'b', 0})},
			expectedErr: "args invalid: contains NUL character",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sys, err := NewSysContext(
				tc.maxSize, // max
				tc.args,
				nil,                              // environ
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              // stdout
				nil,                              // stderr
				nil,                              // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.args, sys.Args())
				require.Equal(t, tc.expectedSize, sys.ArgsSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestNewSysContext_Environ(t *testing.T) {
	tests := []struct {
		name         string
		environ      []string
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			environ:      []string{"a=b", "c=de"},
			expectedSize: 9,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			environ:     []string{"a=b", "c=de"},
			expectedErr: "environ invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			environ:     []string{"a=b", "c=de"},
			expectedErr: "environ invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			environ:     []string{"a=b", string(append([]byte("c=d"), 0))},
			expectedErr: "environ invalid: contains NUL character",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sys, err := NewSysContext(
				tc.maxSize, // max
				nil,        // args
				tc.environ,
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              // stdout
				nil,                              // stderr
				nil,                              // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.environ, sys.Environ())
				require.Equal(t, tc.expectedSize, sys.EnvironSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestSysContext_Close(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		sys := DefaultSysContext()
		require.NoError(t, sys.Close())
	})

	t.Run("open files", func(t *testing.T) {
		tempDir := t.TempDir()
		pathName := "test"
		file, testFS := createWriteableFile(t, tempDir, pathName, make([]byte, 0))

		sys, err := NewSysContext(
			0,   // max
			nil, // args
			nil, // environ
			nil, // stdin
			nil, // stdout
			nil, // stderr
			map[uint32]*FileEntry{ // openedFiles
				3: {Path: "/", FS: testFS},
				4: {Path: ".", FS: testFS},
				5: {Path: path.Join(".", pathName), File: file, FS: testFS},
			},
		)
		require.NoError(t, err)

		// Closing should delete the file descriptors after closing the files.
		require.NoError(t, sys.Close())
		require.Empty(t, sys.openedFiles)

		// Verify it was actually closed, by trying to close it again.
		err = file.(*os.File).Close()
		require.Contains(t, err.Error(), "file already closed")

		// No problem closing config again because the descriptors were removed, so they won't be called again.
		require.NoError(t, sys.Close())
	})

	t.Run("FS never used", func(t *testing.T) {
		testFS := fstest.MapFS{}
		sys, err := NewSysContext(
			0,   // max
			nil, // args
			nil, // environ
			nil, // stdin
			nil, // stdout
			nil, // stderr
			map[uint32]*FileEntry{ // no openedFiles
				3: {Path: "/", FS: testFS},
				4: {Path: ".", FS: testFS},
			},
		)
		require.NoError(t, err)

		// Even if there are no open files, the descriptors for the file-system mappings should be removed.
		require.NoError(t, sys.Close())
		require.Empty(t, sys.openedFiles)
	})

	t.Run("open file externally closed", func(t *testing.T) {
		tempDir := t.TempDir()
		pathName := "test"
		file, testFS := createWriteableFile(t, tempDir, pathName, make([]byte, 0))

		sys, err := NewSysContext(
			0,   // max
			nil, // args
			nil, // environ
			nil, // stdin
			nil, // stdout
			nil, // stderr
			map[uint32]*FileEntry{ // openedFiles
				3: {Path: "/", FS: testFS},
				4: {Path: ".", FS: testFS},
				5: {Path: path.Join(".", pathName), File: file, FS: testFS},
			},
		)
		require.NoError(t, err)

		// Close the file externally
		file.Close()

		// Closing should err as it expected to be open
		require.Contains(t, sys.Close().Error(), "file already closed")

		// However, cleanup should still occur.
		require.Empty(t, sys.openedFiles)
	})
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
