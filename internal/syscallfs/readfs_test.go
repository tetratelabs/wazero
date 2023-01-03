package syscallfs

import (
	"io/fs"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestReadFS_MkDir(t *testing.T) {
	dir := t.TempDir()

	testFS := NewReadFS(dirFS(dir))

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewReadFS(dirFS(tmpDir))

	file1 := "file1"
	file1Path := path.Join(tmpDir, file1)
	file1Contents := []byte{1}
	err := os.WriteFile(file1Path, file1Contents, 0o600)
	require.NoError(t, err)

	file2 := "file2"
	file2Path := path.Join(tmpDir, file2)
	file2Contents := []byte{2}
	err = os.WriteFile(file2Path, file2Contents, 0o600)
	require.NoError(t, err)

	err = testFS.Rename(file1, file2)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Rmdir(t *testing.T) {
	dir := t.TempDir()

	testFS := NewReadFS(dirFS(dir))

	name := "rmdir"
	realPath := path.Join(dir, name)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(name)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Unlink(t *testing.T) {
	dir := t.TempDir()

	testFS := NewReadFS(dirFS(dir))

	name := "unlink"
	realPath := path.Join(dir, name)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(name)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestReadFS_Utimes(t *testing.T) {
	tmpDir := t.TempDir()

	testFS := NewReadFS(dirFS(tmpDir))

	testFS_Utimes(t, tmpDir, testFS)
}

func TestReadFS_Open_Read(t *testing.T) {
	tmpDir := t.TempDir()

	testFS := NewReadFS(dirFS(tmpDir))

	testFS_Open_Read(t, tmpDir, testFS)
}
