package syscallfs

import (
	"errors"
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAdapt_MkDir(t *testing.T) {
	dir := t.TempDir()

	testFS := Adapt(os.DirFS(dir))

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(os.DirFS(tmpDir))

	file1 := "file1"
	file1Path := pathutil.Join(tmpDir, file1)
	file1Contents := []byte{1}
	err := os.WriteFile(file1Path, file1Contents, 0o600)
	require.NoError(t, err)

	file2 := "file2"
	file2Path := pathutil.Join(tmpDir, file2)
	file2Contents := []byte{2}
	err = os.WriteFile(file2Path, file2Contents, 0o600)
	require.NoError(t, err)

	err = testFS.Rename(file1, file2)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Rmdir(t *testing.T) {
	dir := t.TempDir()

	testFS := Adapt(os.DirFS(dir))

	path := "rmdir"
	realPath := pathutil.Join(dir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Unlink(t *testing.T) {
	dir := t.TempDir()

	testFS := Adapt(os.DirFS(dir))

	path := "unlink"
	realPath := pathutil.Join(dir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Utimes(t *testing.T) {
	dir := t.TempDir()

	testFS := Adapt(os.DirFS(dir))

	path := "utimes"
	realPath := pathutil.Join(dir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimes(path, 1, 1)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Open_Read(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = pathutil.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))

	testFS := Adapt(os.DirFS(tmpDir))

	testFS_Open_Read(t, tmpDir, testFS)

	t.Run("path outside root invalid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// fs.FS doesn't allow relative path lookups
		require.True(t, errors.Is(err, fs.ErrInvalid))
	})
}
