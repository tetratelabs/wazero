package sysfs

import (
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewReadFS(t *testing.T) {
	tmpDir := t.TempDir()

	// Doesn't double-wrap file systems that are already read-only
	adapted := Adapt(os.DirFS(tmpDir), "/")
	require.Equal(t, adapted, NewReadFS(adapted))
	require.Equal(t, UnimplementedFS{}, NewReadFS(UnimplementedFS{}))

	// Wraps a writeable file system
	writeable, err := NewDirFS(tmpDir, "/tmp")
	require.NoError(t, err)
	readFS := NewReadFS(writeable)
	require.NotEqual(t, writeable, readFS)
	require.Equal(t, writeable.GuestDir(), readFS.GuestDir())
}

func TestReadFS_String(t *testing.T) {
	writeable, err := NewDirFS(".", "/tmp")
	require.NoError(t, err)

	readFS := NewReadFS(writeable)
	require.NotEqual(t, writeable, readFS)
	require.Equal(t, ".:/tmp:ro", readFS.String())
}

func TestReadFS_MkDir(t *testing.T) {
	writeable, err := NewDirFS(t.TempDir(), "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	err = testFS.Mkdir("mkdir", fs.ModeDir)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	writeable, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	file1 := "file1"
	file1Path := pathutil.Join(tmpDir, file1)
	file1Contents := []byte{1}
	err = os.WriteFile(file1Path, file1Contents, 0o600)
	require.NoError(t, err)

	file2 := "file2"
	file2Path := pathutil.Join(tmpDir, file2)
	file2Contents := []byte{2}
	err = os.WriteFile(file2Path, file2Contents, 0o600)
	require.NoError(t, err)

	err = testFS.Rename(file1, file2)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()
	writeable, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	path := "rmdir"
	realPath := pathutil.Join(tmpDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err = testFS.Rmdir(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Unlink(t *testing.T) {
	tmpDir := t.TempDir()
	writeable, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	path := "unlink"
	realPath := pathutil.Join(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err = testFS.Unlink(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Utimes(t *testing.T) {
	tmpDir := t.TempDir()
	writeable, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	path := "utimes"
	realPath := pathutil.Join(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err = testFS.Utimes(path, 1, 1)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Open_Read(t *testing.T) {
	tmpDir := t.TempDir()
	writeable, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)
	testFS := NewReadFS(writeable)

	testOpen_Read(t, tmpDir, testFS)
}

func TestReadFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	// Create a writeable filesystem
	testFS, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)

	// Wrap it as read-only
	testFS = NewReadFS(testFS)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(&testFSAdapter{testFS}))
}
