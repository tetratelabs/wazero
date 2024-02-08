package sysfs

import (
	"io/fs"
	"os"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewReadFS(t *testing.T) {
	tmpDir := t.TempDir()

	// Wraps a sys.FS because it allows access to Write
	adapted := &AdaptFS{FS: os.DirFS(tmpDir)}
	require.NotEqual(t, adapted, &ReadFS{FS: adapted})

	// Wraps a writeable file system
	writeable := DirFS(tmpDir)
	readFS := &ReadFS{FS: writeable}
	require.NotEqual(t, writeable, readFS)
}

func TestReadFS_Lstat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := DirFS(tmpDir)
	for _, path := range []string{"animals.txt", "sub", "sub-link"} {
		require.EqualErrno(t, 0, writeable.Symlink(path, path+"-link"))
	}

	testFS := &ReadFS{FS: writeable}

	testLstat(t, testFS)
}

func TestReadFS_MkDir(t *testing.T) {
	writeable := DirFS(t.TempDir())
	testFS := &ReadFS{FS: writeable}

	err := testFS.Mkdir("mkdir", fs.ModeDir)
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_Chmod(t *testing.T) {
	writeable := DirFS(t.TempDir())
	testFS := &ReadFS{FS: writeable}

	err := testFS.Chmod("chmod", fs.ModeDir)
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}

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
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}

	path := "rmdir"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.Mkdir(realPath, 0o700))

	err := testFS.Rmdir(path)
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_Unlink(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}

	path := "unlink"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Unlink(path)
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_UtimesNano(t *testing.T) {
	tmpDir := t.TempDir()
	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}

	path := "utimes"
	realPath := joinPath(tmpDir, path)
	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	err := testFS.Utimens(path, sys.UTIME_OMIT, sys.UTIME_OMIT)
	require.EqualErrno(t, sys.EROFS, err)
}

func TestReadFS_Open_Read(t *testing.T) {
	type test struct {
		name          string
		fs            func(tmpDir string) sys.FS
		expectFileIno bool
		expectDirIno  bool
	}

	tests := []test{
		{
			name: "DirFS",
			fs: func(tmpDir string) sys.FS {
				return DirFS(tmpDir)
			},
			expectFileIno: true,
			expectDirIno:  true,
		},
		{
			name: "fstest.MapFS",
			fs: func(tmpDir string) sys.FS {
				return &AdaptFS{FS: fstest.FS}
			},
			expectFileIno: false,
			expectDirIno:  false,
		},
		{
			name: "os.DirFS",
			fs: func(tmpDir string) sys.FS {
				return &AdaptFS{FS: os.DirFS(tmpDir)}
			},
			expectFileIno: true,
			expectDirIno:  runtime.GOOS != "windows",
		},
		{
			name: "mask(os.DirFS)",
			fs: func(tmpDir string) sys.FS {
				return &AdaptFS{FS: &MaskOsFS{Fs: os.DirFS(tmpDir)}}
			},
			expectFileIno: true,
			expectDirIno:  runtime.GOOS != "windows",
		},
		{
			name: "mask(os.DirFS) ZeroIno",
			fs: func(tmpDir string) sys.FS {
				return &AdaptFS{FS: &MaskOsFS{Fs: os.DirFS(tmpDir), ZeroIno: true}}
			},
			expectFileIno: false,
			expectDirIno:  false,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Ensure tests don't conflict on the same directory
			tmpDir := t.TempDir()
			require.NoError(t, fstest.WriteTestFiles(tmpDir))

			testOpen_Read(t, &ReadFS{FS: tc.fs(tmpDir)}, tc.expectFileIno, tc.expectDirIno)
		})
	}
}

func TestReadFS_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}
	testStat(t, testFS)
}

func TestReadFS_Readlink(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	writeable := DirFS(tmpDir)
	testFS := &ReadFS{FS: writeable}
	testReadlink(t, testFS, writeable)
}
