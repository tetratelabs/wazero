package sys

import (
	"context"
	"embed"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

var (
	noopStdin  = &FileEntry{Name: "stdin", File: NewStdioFileReader(eofReader{}, noopStdinStat, PollerDefaultStdin)}
	noopStdout = &FileEntry{Name: "stdout", File: &stdioFileWriter{w: io.Discard, s: noopStdoutStat}}
	noopStderr = &FileEntry{Name: "stderr", File: &stdioFileWriter{w: io.Discard, s: noopStderrStat}}
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
			c := Context{}
			err := c.NewFSContext(nil, nil, nil, tc.fs)
			require.NoError(t, err)
			fsc := c.fsc
			defer fsc.Close(testCtx)

			preopenedDir, _ := fsc.openedFiles.Lookup(FdPreopen)
			require.Equal(t, tc.fs, fsc.rootFS)
			require.NotNil(t, preopenedDir)
			require.Equal(t, "/", preopenedDir.Name)

			// Verify that each call to OpenFile returns a different file
			// descriptor.
			f1, errno := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.Zero(t, errno)
			require.NotEqual(t, FdPreopen, f1)

			// Verify that file descriptors are reused.
			//
			// Note that this specific behavior is not required by WASI which
			// only documents that file descriptor numbers will be selected
			// randomly and applications should not rely on them. We added this
			// test to ensure that our implementation properly reuses descriptor
			// numbers but if we were to change the reuse strategy, this test
			// would likely break and need to be updated.
			require.Zero(t, fsc.CloseFile(f1))
			f2, errno := fsc.OpenFile(preopenedDir.FS, preopenedDir.Name, 0, 0)
			require.Zero(t, errno)
			require.Equal(t, f1, f2)
		})
	}
}

func TestFSContext_CloseFile(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)
	testFS := sysfs.Adapt(embedFS)

	c := Context{}
	err = c.NewFSContext(nil, nil, nil, testFS)
	require.NoError(t, err)
	fsc := c.fsc
	defer fsc.Close(testCtx)

	fdToClose, errno := fsc.OpenFile(testFS, "empty.txt", os.O_RDONLY, 0)
	require.Zero(t, errno)

	fdToKeep, errno := fsc.OpenFile(testFS, "test.txt", os.O_RDONLY, 0)
	require.Zero(t, errno)

	// Close
	require.Zero(t, fsc.CloseFile(fdToClose))

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.LookupFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.LookupFile(fdToKeep)
	require.True(t, ok)

	t.Run("EBADF for an invalid FD", func(t *testing.T) {
		require.EqualErrno(t, syscall.EBADF, fsc.CloseFile(42)) // 42 is an arbitrary invalid FD
	})
	t.Run("Can close a pre-open", func(t *testing.T) {
		require.Zero(t, fsc.CloseFile(FdPreopen))
	})
}

func TestUnimplementedFSContext(t *testing.T) {
	c := Context{}
	err := c.NewFSContext(nil, nil, nil, sysfs.UnimplementedFS{})
	require.NoError(t, err)
	testFS := &c.fsc
	require.NoError(t, err)

	expected := &FSContext{rootFS: sysfs.UnimplementedFS{}}
	expected.openedFiles.Insert(noopStdin)
	expected.openedFiles.Insert(noopStdout)
	expected.openedFiles.Insert(noopStderr)

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close(testCtx)
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{rootFS: sysfs.UnimplementedFS{}}, testFS)
	})
}

func TestCompositeFSContext(t *testing.T) {
	tmpDir1 := t.TempDir()
	testFS1 := sysfs.NewDirFS(tmpDir1)

	tmpDir2 := t.TempDir()
	testFS2 := sysfs.NewDirFS(tmpDir2)

	rootFS, err := sysfs.NewRootFS([]sysfs.FS{testFS2, testFS1}, []string{"/tmp", "/"})
	require.NoError(t, err)

	c := Context{}
	err = c.NewFSContext(nil, nil, nil, rootFS)
	require.NoError(t, err)
	testFS := &c.fsc

	// Ensure the pre-opens have exactly the name specified, and are in order.
	preopen3, ok := testFS.openedFiles.Lookup(3)
	require.True(t, ok)
	require.Equal(t, "/tmp", preopen3.Name)
	preopen4, ok := testFS.openedFiles.Lookup(4)
	require.True(t, ok)
	require.Equal(t, "/", preopen4.Name)

	t.Run("Close closes", func(t *testing.T) {
		err := testFS.Close(testCtx)
		require.NoError(t, err)

		// Closes opened files
		require.Equal(t, &FSContext{rootFS: rootFS}, testFS)
	})
}

func TestContext_Close(t *testing.T) {
	testFS := sysfs.Adapt(testfs.FS{"foo": &testfs.File{}})

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, testFS)
	require.NoError(t, err)
	fsc := c.fsc

	// Verify base case
	require.Equal(t, 1+FdPreopen, uint32(fsc.openedFiles.Len()))

	_, errno := fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.Zero(t, errno)
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

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, testFS)
	require.NoError(t, err)
	fsc := c.fsc

	// open another file
	_, errno := fsc.OpenFile(testFS, "foo", os.O_RDONLY, 0)
	require.Zero(t, errno)

	require.EqualError(t, fsc.Close(testCtx), "error closing")

	// Paths should clear even under error
	require.Zero(t, fsc.openedFiles.Len(), "expected no opened files")
}

func TestFSContext_ReOpenDir(t *testing.T) {
	tmpDir := t.TempDir()
	dirFs := sysfs.NewDirFS(tmpDir)

	const dirName = "dir"
	errno := dirFs.Mkdir(dirName, 0o700)
	require.Zero(t, errno)

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, dirFs)
	require.NoError(t, err)
	fsc := c.fsc

	require.NoError(t, err)
	defer func() {
		require.NoError(t, fsc.Close(context.Background()))
	}()

	t.Run("ok", func(t *testing.T) {
		dirFd, errno := fsc.OpenFile(dirFs, dirName, os.O_RDONLY, 0o600)
		require.Zero(t, errno)

		ent, ok := fsc.LookupFile(dirFd)
		require.True(t, ok)

		// Set arbitrary state.
		ent.ReadDir = &ReadDir{Dirents: make([]*platform.Dirent, 10), CountRead: 12345}

		// Then reopen the same file descriptor.
		ent, errno = fsc.ReOpenDir(dirFd)
		require.Zero(t, errno)

		// Verify the read dir state has been reset.
		require.Equal(t, &ReadDir{}, ent.ReadDir)
	})

	t.Run("non existing ", func(t *testing.T) {
		_, errno = fsc.ReOpenDir(12345)
		require.EqualErrno(t, syscall.EBADF, errno)
	})

	t.Run("not dir", func(t *testing.T) {
		const fileName = "dog"
		fd, errno := fsc.OpenFile(dirFs, fileName, os.O_CREATE, 0o600)
		require.Zero(t, errno)

		_, errno = fsc.ReOpenDir(fd)
		require.EqualErrno(t, syscall.EISDIR, errno)
	})
}

func TestFSContext_Renumber(t *testing.T) {
	tmpDir := t.TempDir()
	dirFs := sysfs.NewDirFS(tmpDir)

	const dirName = "dir"
	errno := dirFs.Mkdir(dirName, 0o700)
	require.Zero(t, errno)

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, dirFs)
	require.NoError(t, err)
	fsc := c.fsc

	defer func() {
		require.NoError(t, fsc.Close(context.Background()))
	}()

	for _, toFd := range []uint32{10, 100, 100} {
		fromFd, errno := fsc.OpenFile(dirFs, dirName, os.O_RDONLY, 0)
		require.Zero(t, errno)

		prevDirFile, ok := fsc.LookupFile(fromFd)
		require.True(t, ok)

		require.Zero(t, fsc.Renumber(fromFd, toFd))

		renumberedDirFile, ok := fsc.LookupFile(toFd)
		require.True(t, ok)

		require.Equal(t, prevDirFile, renumberedDirFile)

		// Previous file descriptor shouldn't be used.
		_, ok = fsc.LookupFile(fromFd)
		require.False(t, ok)
	}

	t.Run("errors", func(t *testing.T) {
		// Sanity check for 3 being preopen.
		preopen, ok := fsc.LookupFile(3)
		require.True(t, ok)
		require.True(t, preopen.IsPreopen)

		// From is preopen.
		require.Equal(t, syscall.ENOTSUP, fsc.Renumber(3, 100))

		// From does not exist.
		require.Equal(t, syscall.EBADF, fsc.Renumber(12345, 3))

		// Both are preopen.
		require.Equal(t, syscall.ENOTSUP, fsc.Renumber(3, 3))
	})
}

func TestFSContext_ChangeOpenFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dirFs := sysfs.NewDirFS(tmpDir)

	const fileName = "dir"
	require.NoError(t, os.WriteFile(path.Join(tmpDir, fileName), []byte("0123456789"), 0o600))

	c := Context{}
	err := c.NewFSContext(nil, nil, nil, dirFs)
	require.NoError(t, err)
	fsc := c.fsc

	defer func() {
		require.NoError(t, fsc.Close(context.Background()))
	}()

	// Without APPEND.
	fd, errno := fsc.OpenFile(dirFs, fileName, os.O_RDWR, 0o600)
	require.Zero(t, errno)

	f0, ok := fsc.openedFiles.Lookup(fd)
	require.True(t, ok)
	require.Equal(t, f0.openFlag&syscall.O_APPEND, 0)

	// Set the APPEND flag.
	require.Zero(t, fsc.ChangeOpenFlag(fd, syscall.O_APPEND))
	f1, ok := fsc.openedFiles.Lookup(fd)
	require.True(t, ok)
	require.Equal(t, f1.openFlag&syscall.O_APPEND, syscall.O_APPEND)

	// Remove the APPEND flag.
	require.Zero(t, fsc.ChangeOpenFlag(fd, 0))
	f2, ok := fsc.openedFiles.Lookup(fd)
	require.True(t, ok)
	require.Equal(t, f2.openFlag&syscall.O_APPEND, 0)
}

func TestWriterForFile(t *testing.T) {
	c := Context{}
	err := c.NewFSContext(nil, nil, nil, sysfs.UnimplementedFS{})
	require.NoError(t, err)
	testFS := &c.fsc

	require.Nil(t, WriterForFile(testFS, FdStdin))
	require.Equal(t, noopStdout.File, WriterForFile(testFS, FdStdout))
	require.Equal(t, noopStderr.File, WriterForFile(testFS, FdStderr))
	require.Nil(t, WriterForFile(testFS, FdPreopen))
}

func TestStdioStat(t *testing.T) {
	stat, err := stdioStat(os.Stdin, noopStdinStat)
	require.NoError(t, err)
	stdinStatMode := stat.Mode()
	// ensure we are consistent with sys stdin
	osStdinStat, _ := os.Stdin.Stat()
	osStdinMode := osStdinStat.Mode().Type()
	require.Equal(t, osStdinMode&fs.ModeDevice, stdinStatMode&fs.ModeDevice)
	require.Equal(t, osStdinMode&fs.ModeCharDevice, stdinStatMode&fs.ModeCharDevice)

	stat, err = stdioStat(os.Stdout, noopStdoutStat)
	stdoutStatMode := stat.Mode()
	require.NoError(t, err)
	// ensure we are consistent with sys stdout
	osStdoutStat, _ := os.Stdout.Stat()
	osStdoutMode := osStdoutStat.Mode().Type()
	require.Equal(t, osStdoutMode&fs.ModeDevice, stdoutStatMode&fs.ModeDevice)
	require.Equal(t, osStdoutMode&fs.ModeCharDevice, stdoutStatMode&fs.ModeCharDevice)

	stat, err = stdioStat(os.Stderr, noopStderrStat)
	require.NoError(t, err)
	stderrStatMode := stat.Mode()
	// ensure we are consistent with sys stderr
	osStderrStat, _ := os.Stderr.Stat()
	osStderrMode := osStderrStat.Mode().Type()
	require.Equal(t, osStderrMode&fs.ModeDevice, stderrStatMode&fs.ModeDevice)
	require.Equal(t, osStderrMode&fs.ModeCharDevice, stderrStatMode&fs.ModeCharDevice)

	// simulate regular file attached to stdin
	f, err := os.CreateTemp("", "somefile")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(f.Name()) // clean up
	stat, err = stdioStat(f, noopStdinStat)
	require.NoError(t, err)
	fStat := stat.Mode()
	osFStat, _ := f.Stat()
	osFStatMode := osFStat.Mode()
	require.Equal(t, osFStatMode&fs.ModeDevice, fStat&fs.ModeDevice)
	require.Equal(t, osFStatMode&fs.ModeCharDevice, fStat&fs.ModeCharDevice)

	// interface{} returns default
	stat, err = stdioStat("whatevs", noopStdinStat)
	require.NoError(t, err)
	require.Equal(t, noopStdinStat, stat)

	// nil *File returns err
	var nilFile *os.File
	_, err = stdioStat(nilFile, noopStdinStat)
	require.Error(t, err)
}
