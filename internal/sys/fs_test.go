package sys

import (
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

var (
	noopStdin  = &FileEntry{IsPreopen: true, Name: "stdin", File: &stdioFileReader{r: eofReader{}, s: noopStdinStat}}
	noopStdout = &FileEntry{IsPreopen: true, Name: "stdout", File: &stdioFileWriter{w: io.Discard, s: noopStdoutStat}}
	noopStderr = &FileEntry{IsPreopen: true, Name: "stderr", File: &stdioFileWriter{w: io.Discard, s: noopStderrStat}}
)

//go:embed testdata
var testdata embed.FS

func TestNewFSContext(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	dirfs := sysfs.NewDirFS(".")

	// Test various usual configuration for the file system.
	tests := []struct {
		name string
		fs   sysfs.FS
	}{
		{
			name: "embed.FS",
			fs:   sysfs.Adapt(embedFS),
		},
		{
			name: "sysfs.NewDirFS",
			// Don't use "testdata" because it may not be present in
			// cross-architecture (a.k.a. scratch) build containers.
			fs: dirfs,
		},
		{
			name: "sysfs.NewReadFS",
			fs:   sysfs.NewReadFS(dirfs),
		},
		{
			name: "fstest.MapFS",
			fs:   sysfs.Adapt(fstest.MapFS{}),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(b *testing.T) {
			fsc, err := NewFSContext(nil, nil, nil, tc.fs)
			require.NoError(t, err)
			defer fsc.Close(testCtx)

			preopenedDir, _ := fsc.openedFiles.Lookup(FdPreopen)
			require.Equal(t, tc.fs, fsc.root)
			require.NotNil(t, preopenedDir)
			require.Equal(t, "/", preopenedDir.Name)

			// Verify that each call to OpenFile returns a different file
			// descriptor.
			f1, err := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.NoError(t, err)
			require.NotEqual(t, FdPreopen, f1)

			// Verify that file descriptors are reused.
			//
			// Note that this specific behavior is not required by WASI which
			// only documents that file descriptor numbers will be selected
			// randomly and applications should not rely on them. We added this
			// test to ensure that our implementation properly reuses descriptor
			// numbers but if we were to change the reuse strategy, this test
			// would likely break and need to be updated.
			require.NoError(t, fsc.CloseFile(f1))
			f2, err := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.NoError(t, err)
			require.Equal(t, f1, f2)
		})
	}
}

func TestUnimplementedFSContext(t *testing.T) {
	testFS, err := NewFSContext(nil, nil, nil, sysfs.UnimplementedFS{})
	require.NoError(t, err)

	expected := &FSContext{root: sysfs.UnimplementedFS{}}
	expected.openedFiles.Insert(noopStdin)
	expected.openedFiles.Insert(noopStdout)
	expected.openedFiles.Insert(noopStderr)

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close(testCtx)
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{root: sysfs.UnimplementedFS{}}, testFS)
	})
}

func TestContext_Close(t *testing.T) {
	testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{}})

	fsc, err := NewFSContext(nil, nil, nil, testFS)
	require.NoError(t, err)

	// Verify base case
	require.Equal(t, 1+FdPreopen, uint32(fsc.openedFiles.Len()))

	_, err = fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.NoError(t, err)
	require.Equal(t, 2+FdPreopen, uint32(fsc.openedFiles.Len()))

	// Closing should not err.
	require.NoError(t, fsc.Close(testCtx))

	// Verify our intended side-effect
	require.Zero(t, fsc.openedFiles.Len())

	// Verify no error closing again.
	require.NoError(t, fsc.Close(testCtx))
}

func TestContext_Close_Error(t *testing.T) {
	file := &testfs.File{CloseErr: errors.New("error closing")}

	testFS := sysfs.Adapt(testfs.FS{"foo": file})

	fsc, err := NewFSContext(nil, nil, nil, testFS)
	require.NoError(t, err)

	// open another file
	_, err = fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.NoError(t, err)

	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, fsc.openedFiles.Len(), "expected no opened files")
}
