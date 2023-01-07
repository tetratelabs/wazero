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
	testFS := Adapt(os.DirFS(t.TempDir()))

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Rename(t *testing.T) {
	rootDir := t.TempDir()
	testFS := Adapt(os.DirFS(rootDir))

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

func TestAdapt_Rmdir(t *testing.T) {
	rootDir := t.TempDir()
	testFS := Adapt(os.DirFS(rootDir))

	path := "rmdir"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Unlink(t *testing.T) {
	rootDir := t.TempDir()
	testFS := Adapt(os.DirFS(rootDir))

	path := "unlink"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Utimes(t *testing.T) {
	rootDir := t.TempDir()
	testFS := Adapt(os.DirFS(rootDir))

	path := "utimes"
	realPath := pathutil.Join(rootDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimes(path, 1, 1)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestAdapt_Open_Read(t *testing.T) {
	// Create a subdirectory, so we can test reads outside the FS root.
	rootDir := t.TempDir()
	rootDir = pathutil.Join(rootDir, t.Name())
	require.NoError(t, os.Mkdir(rootDir, 0o700))
	testFS := Adapt(os.DirFS(rootDir))

	testOpen_Read(t, rootDir, testFS)

	t.Run("path outside root invalid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// fs.FS doesn't allow relative path lookups
		require.True(t, errors.Is(err, fs.ErrInvalid))
	})
}

// hackFS cheats the fs.FS contract by opening for write (os.O_RDWR).
//
// Until we have an alternate public interface for filesystems, some users will
// rely on this. Via testing, we ensure we don't accidentally break them.
type hackFS string

func (dir hackFS) Open(name string) (fs.File, error) {
	path := ensureTrailingPathSeparator(string(dir)) + name

	if f, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
		return f, nil
	} else if errors.Is(err, syscall.EISDIR) {
		return os.OpenFile(path, os.O_RDONLY, 0)
	} else {
		return nil, err
	}
}

// TestAdapt_HackedWrites ensures we allow writes even if they violate the
// fs.FS contract.
func TestAdapt_HackedWrites(t *testing.T) {
	rootDir := t.TempDir()
	testFS := Adapt(hackFS(rootDir))

	testOpen_O_RDWR(t, rootDir, testFS)
}
