package sysfs

import (
	"io/fs"
	"os"
	"runtime"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewReadFS(t *testing.T) {
	tmpDir := t.TempDir()

	// Doesn't double-wrap file systems that are already read-only
	require.Equal(t, UnimplementedFS{}, NewReadFS(UnimplementedFS{}))

	// Wraps a fs.FS because it allows access to Write
	adapted := Adapt(os.DirFS(tmpDir))
	require.NotEqual(t, adapted, NewReadFS(adapted))

	// Wraps a writeable file system
	writeable := NewDirFS(tmpDir)
	readFS := NewReadFS(writeable)
	require.NotEqual(t, writeable, readFS)
}

func TestReadFS_String(t *testing.T) {
	writeable := NewDirFS("/tmp")

	readFS := NewReadFS(writeable)
	require.NotEqual(t, writeable, readFS)
	require.Equal(t, "/tmp", readFS.String())
}

func TestReadFS_Lstat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := NewDirFS(tmpDir)
	for _, path := range []string{"animals.txt", "sub", "sub-link"} {
		require.Zero(t, writeable.Symlink(path, path+"-link"))
	}

	testFS := NewReadFS(writeable)

	testLstat(t, testFS)
}

func TestReadFS_MkDir(t *testing.T) {
	writeable := NewDirFS(t.TempDir())
	testFS := NewReadFS(writeable)

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_Chmod(t *testing.T) {
	writeable := NewDirFS(t.TempDir())
	testFS := NewReadFS(writeable)

	err := testFS.Chmod("chmod", fs.ModeDir)
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)

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
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)

	path := "rmdir"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_Unlink(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)

	path := "unlink"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_UtimesNano(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)

	path := "utimes"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimens(path, nil, true)
	require.EqualErrno(t, syscall.EROFS, err)
}

func TestReadFS_Open_Read(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	type test struct {
		name      string
		fs        FS
		expectIno bool
	}

	tests := []test{
		{name: "DirFS", fs: NewReadFS(NewDirFS(tmpDir)), expectIno: true},
		{name: "fstest.MapFS", fs: NewReadFS(Adapt(fstest.FS)), expectIno: false},
	}

	// We can't correct operating system portability issues with os.DirFS on
	// windows. Use syscall.DirFS instead!
	if runtime.GOOS != "windows" {
		tests = append(tests, test{name: "os.DirFS", fs: NewReadFS(Adapt(os.DirFS(tmpDir))), expectIno: true})
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			testOpen_Read(t, tc.fs, tc.expectIno)
		})
	}
}

func TestReadFS_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)
	testStat(t, testFS)
}

func TestReadFS_Readlink(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := NewDirFS(tmpDir)
	testFS := NewReadFS(writeable)
	testReadlink(t, testFS, writeable)
}

func TestReadFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	// Create a writeable filesystem
	testFS := NewDirFS(tmpDir)

	// Wrap it as read-only
	testFS = NewReadFS(testFS)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(testFS.(fs.FS)))
}
