package syscallfs

import (
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReadFS_MkDir(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rename(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	file1 := "file1"
	file1Path := pathutil.Join(rootDir, file1)
	file1Contents := []byte{1}
	err := os.WriteFile(file1Path, file1Contents, 0o600)
	require.NoError(t, err)

	file2 := "file2"
	file2Path := pathutil.Join(rootDir, file2)
	file2Contents := []byte{2}
	err = os.WriteFile(file2Path, file2Contents, 0o600)
	require.NoError(t, err)

	err = testFS.Rename(file1, file2)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rmdir(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	path := "rmdir"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Unlink(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	path := "unlink"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Utimes(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	path := "utimes"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimes(path, 1, 1)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Open_Read(t *testing.T) {
	rootDir := t.TempDir()
	testFS := NewReadFS(Adapt(hackFS(rootDir)))

	testOpen_Read(t, rootDir, testFS)
}
