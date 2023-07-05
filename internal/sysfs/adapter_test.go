package sysfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAdapt_nil(t *testing.T) {
	testFS := Adapt(nil)
	_, ok := testFS.(fsapi.UnimplementedFS)
	require.True(t, ok)
}

func TestAdapt_String(t *testing.T) {
	testFS := Adapt(os.DirFS("."))
	require.Equal(t, ".", testFS.String())
}

func TestAdapt_MkDir(t *testing.T) {
	testFS := Adapt(os.DirFS(t.TempDir()))

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_Chmod(t *testing.T) {
	testFS := Adapt(os.DirFS(t.TempDir()))

	err := testFS.Chmod("chmod", fs.ModeDir)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(os.DirFS(tmpDir))

	file1 := "file1"
	file1Path := joinPath(tmpDir, file1)
	file1Contents := []byte{1}
	err := os.WriteFile(file1Path, file1Contents, 0o600)
	require.NoError(t, err)

	file2 := "file2"
	file2Path := joinPath(tmpDir, file2)
	file2Contents := []byte{2}
	err = os.WriteFile(file2Path, file2Contents, 0o600)
	require.NoError(t, err)

	err = testFS.Rename(file1, file2)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(os.DirFS(tmpDir))

	path := "rmdir"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_Unlink(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(os.DirFS(tmpDir))

	path := "unlink"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_UtimesNano(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(os.DirFS(tmpDir))

	path := "utimes"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimens(path, nil, true)
	require.EqualErrno(t, syscall.ENOSYS, err)
}

func TestAdapt_Open_Read(t *testing.T) {
	// Create a subdirectory, so we can test reads outside the fsapi.FS root.
	tmpDir := t.TempDir()
	tmpDir = joinPath(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	testFS := Adapt(os.DirFS(tmpDir))

	// We can't correct operating system portability issues with os.DirFS on
	// windows. Use syscall.DirFS instead!
	testOpen_Read(t, testFS, true, runtime.GOOS != "windows")

	t.Run("path outside root invalid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// fsapi.FS doesn't allow relative path lookups
		require.EqualErrno(t, syscall.EINVAL, err)
	})
}

func TestAdapt_Lstat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	testFS := Adapt(os.DirFS(tmpDir))

	for _, path := range []string{"animals.txt", "sub", "sub-link"} {
		fullPath := joinPath(tmpDir, path)
		linkPath := joinPath(tmpDir, path+"-link")
		require.NoError(t, os.Symlink(fullPath, linkPath))

		_, errno := testFS.Lstat(filepath.Base(linkPath))
		require.EqualErrno(t, 0, errno)
	}
}

func TestAdapt_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := Adapt(os.DirFS(tmpDir))
	testStat(t, testFS)
}

// hackFS cheats the api.FS contract by opening for write (os.O_RDWR).
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
	} else if errors.Is(err, syscall.ENOENT) {
		return os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o444)
	} else {
		return nil, err
	}
}

// TestAdapt_HackedWrites ensures we allow writes even if they violate the
// api.FS contract.
func TestAdapt_HackedWrites(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := Adapt(hackFS(tmpDir))

	testOpen_O_RDWR(t, tmpDir, testFS)
}

// MaskOsFS helps prove that the fsFile implementation behaves the same way
// when a fs.FS returns an os.File or methods we use from it.
type MaskOsFS struct {
	Fs fs.FS
}

func (f *MaskOsFS) Open(name string) (fs.File, error) {
	if f, err := f.Fs.Open(name); err != nil {
		return nil, err
	} else if osF, ok := f.(*os.File); !ok {
		return nil, fmt.Errorf("input not an os.File %v", osF)
	} else {
		return struct{ methodsUsedByFsAdapter }{osF}, nil
	}
}

// methodsUsedByFsAdapter includes all functions Adapt supports. This includes
// the ability to write files and seek files or directories (directories only
// to zero).
//
// A fs.File implementing this should be functionally equivalent to an os.File,
// even if both are less ideal than using NewDirFS directly, especially on
// Windows.
//
// For example, on Windows, we cannot reliably read the inode for a
// fsapi.Dirent with any of these functions.
type methodsUsedByFsAdapter interface {
	// fs.File is used to implement `stat`, `read` and `close`.
	fs.File

	// Fd is only used on windows, to back-fill the inode on `stat`.
	// When implemented, this should dispatch to the same function on os.File.
	Fd() uintptr

	// io.ReaderAt is used to implement `pread`.
	io.ReaderAt

	// io.Seeker is used to implement `seek` on a file or directory. It is also
	// used to implement `pread` when io.ReaderAt isn't implemented.
	io.Seeker
	// ^-- TODO: we can also use this to backfill support for `pwrite`

	// Readdir is used to implement `readdir`, and attempts to retrieve inodes.
	// When implemented, this should dispatch to the same function on os.File.
	Readdir(n int) ([]fs.FileInfo, error)

	// Readdir is used to implement `readdir` when Readdir is not available.
	fs.ReadDirFile

	// io.Writer is used to implement `write`.
	io.Writer

	// io.WriterAt is used to implement `pwrite`.
	io.WriterAt
}
