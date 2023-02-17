package sysfs

import (
	"errors"
	"io/fs"
	"os"
	pathutil "path"
	"runtime"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewDirFS(t *testing.T) {
	testFS := NewDirFS(".")

	// Guest can look up /
	f, err := testFS.OpenFile("/", os.O_RDONLY, 0)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	t.Run("host path not found", func(t *testing.T) {
		testFS := NewDirFS("a")
		_, err = testFS.OpenFile(".", os.O_RDONLY, 0)
		require.Equal(t, syscall.ENOENT, err)
	})
	t.Run("host path not a directory", func(t *testing.T) {
		arg0 := os.Args[0] // should be safe in scratch tests which don't have the source mounted.

		testFS := NewDirFS(arg0)
		d, err := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.NoError(t, err)
		_, err = d.(fs.ReadDirFile).ReadDir(-1)
		require.Equal(t, syscall.ENOTDIR, errors.Unwrap(err))
	})
}

func TestDirFS_String(t *testing.T) {
	testFS := NewDirFS(".")

	// String has the name of the path entered
	require.Equal(t, ".", testFS.String())
}

func TestDirFS_MkDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	name := "mkdir"
	realPath := pathutil.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		require.NoError(t, testFS.Mkdir(name, fs.ModeDir))
		stat, err := os.Stat(realPath)
		require.NoError(t, err)
		require.Equal(t, name, stat.Name())
		require.True(t, stat.IsDir())
	})

	t.Run("dir exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, err)
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.Remove(realPath))
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Mkdir(name, fs.ModeDir)
		require.Equal(t, syscall.EEXIST, err)
	})
	t.Run("try creating on file", func(t *testing.T) {
		filePath := pathutil.Join("non-existing-dir", "foo.txt")
		err := testFS.Mkdir(filePath, fs.ModeDir)
		require.Equal(t, syscall.ENOENT, err)
	})

	// Remove the path so that we can test creating it with perms.
	require.NoError(t, os.Remove(realPath))

	// Setting mode only applies to files on windows
	if runtime.GOOS != "windows" {
		t.Run("dir", func(t *testing.T) {
			require.NoError(t, os.Mkdir(realPath, 0o444))
			defer os.RemoveAll(realPath)
			testChmod(t, testFS, name)
		})
	}

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, nil, 0o444))
		defer os.RemoveAll(realPath)
		testChmod(t, testFS, name)
	})
}

func testChmod(t *testing.T, testFS FS, path string) {
	// test base case
	requireMode(t, testFS, path, 0o444)

	// test we can mark owner writeable
	require.NoError(t, testFS.Chmod(path, 0o666))
	requireMode(t, testFS, path, 0o666)

	if runtime.GOOS != "windows" {
		// test we can mark owner executable
		require.NoError(t, testFS.Chmod(path, 0o500))
		requireMode(t, testFS, path, 0o500)
	}
}

func requireMode(t *testing.T, testFS FS, path string, mode fs.FileMode) {
	stat, err := StatPath(testFS, path)
	require.NoError(t, err)
	require.Equal(t, mode, stat.Mode()&fs.ModePerm)
}

func TestDirFS_Rename(t *testing.T) {
	t.Run("from doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		err := os.WriteFile(file1Path, []byte{1}, 0o600)
		require.NoError(t, err)

		err = testFS.Rename("file2", file1)
		require.Equal(t, syscall.ENOENT, err)
	})
	t.Run("file to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		file2 := "file2"
		file2Path := pathutil.Join(tmpDir, file2)
		err = testFS.Rename(file1, file2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		s, err := os.Stat(file2Path)
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("dir to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)
		err := testFS.Rename(dir1, dir2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		s, err := os.Stat(dir2Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("dir to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)

		// write a file to that path
		f, err := os.OpenFile(dir2Path, os.O_RDWR|os.O_CREATE, 0o600)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		err = testFS.Rename(dir1, dir2)
		require.Equal(t, syscall.ENOTDIR, err)
	})
	t.Run("file to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err = testFS.Rename(file1, dir1)
		require.Equal(t, syscall.EISDIR, err)
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L567-L582
	t.Run("dir to empty dir should be fine", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := pathutil.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		err = testFS.Rename(dir1, dir2)
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		// Show the file inside that directory moved
		s, err := os.Stat(pathutil.Join(dir2Path, file1))
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L584-L604
	t.Run("dir to non empty dir should be EXIST", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := pathutil.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := pathutil.Join(tmpDir, dir2)
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		// Make the destination non-empty.
		err = os.WriteFile(pathutil.Join(dir2Path, "existing.txt"), []byte("any thing"), 0o600)
		require.NoError(t, err)

		err = testFS.Rename(dir1, dir2)
		require.ErrorIs(t, syscall.ENOTEMPTY, err)
	})

	t.Run("file to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

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
		require.NoError(t, err)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.Equal(t, syscall.ENOENT, errors.Unwrap(err))

		// Show the file1 overwrote file2
		b, err := os.ReadFile(file2Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
	t.Run("dir to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := pathutil.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		err := testFS.Rename(dir1, dir1)
		require.NoError(t, err)

		s, err := os.Stat(dir1Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("file to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := pathutil.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		err = testFS.Rename(file1, file1)
		require.NoError(t, err)

		b, err := os.ReadFile(file1Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
}

func TestDirFS_Rmdir(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	name := "rmdir"
	realPath := pathutil.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("dir not empty", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))
		fileInDir := pathutil.Join(realPath, "file")
		require.NoError(t, os.WriteFile(fileInDir, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTEMPTY, err)

		require.NoError(t, os.Remove(fileInDir))
	})

	t.Run("dir empty", func(t *testing.T) {
		require.NoError(t, testFS.Rmdir(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})

	t.Run("not directory", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.Equal(t, syscall.ENOTDIR, err)

		require.NoError(t, os.Remove(realPath))
	})
}

func TestDirFS_Unlink(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	name := "unlink"
	realPath := pathutil.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Unlink(name)
		require.Equal(t, syscall.ENOENT, err)
	})

	t.Run("not file", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Unlink(name)
		require.Equal(t, syscall.EISDIR, err)

		require.NoError(t, os.Remove(realPath))
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		require.NoError(t, testFS.Unlink(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})
}

func TestDirFS_Utimes(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	testUtimes(t, tmpDir, testFS)
}

func TestDirFS_OpenFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = pathutil.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))

	testFS := NewDirFS(tmpDir)

	testOpen_Read(t, tmpDir, testFS)

	testOpen_O_RDWR(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// syscall.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestDirFS_Truncate(t *testing.T) {
	content := []byte("123456")

	tests := []struct {
		name            string
		size            int64
		expectedContent []byte
		expectedErr     error
	}{
		{
			name:            "one less",
			size:            5,
			expectedContent: []byte("12345"),
		},
		{
			name:            "same",
			size:            6,
			expectedContent: content,
		},
		{
			name:            "zero",
			size:            0,
			expectedContent: []byte(""),
		},
		{
			name:            "larger",
			size:            106,
			expectedContent: append(content, make([]byte, 100)...),
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFS := NewDirFS(tmpDir)

			name := "truncate"
			realPath := pathutil.Join(tmpDir, name)
			require.NoError(t, os.WriteFile(realPath, content, 0o0600))

			err := testFS.Truncate(name, tc.size)
			require.NoError(t, err)

			actual, err := os.ReadFile(realPath)
			require.NoError(t, err)
			require.Equal(t, tc.expectedContent, actual)
		})
	}

	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	name := "truncate"
	realPath := pathutil.Join(tmpDir, name)

	if runtime.GOOS != "windows" {
		// TODO: os.Truncate on windows can create the file even when it
		// doesn't exist.
		t.Run("doesn't exist", func(t *testing.T) {
			err := testFS.Truncate(name, 0)
			require.Equal(t, syscall.ENOENT, err)
		})
	}

	t.Run("not file", func(t *testing.T) {
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Truncate(name, 0)
		require.Equal(t, syscall.EISDIR, err)

		require.NoError(t, os.Remove(realPath))
	})

	require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

	t.Run("negative", func(t *testing.T) {
		err := testFS.Truncate(name, -1)
		require.Equal(t, syscall.EINVAL, err)
	})
}

func TestDirFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	// Create a writeable filesystem
	testFS := NewDirFS(tmpDir)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(testFS.(fs.FS)))
}

// Test_fdReaddir_opened_file_written ensures that writing files to the already-opened directory
// is visible. This is significant on Windows.
// https://github.com/ziglang/zig/blob/2ccff5115454bab4898bae3de88f5619310bc5c1/lib/std/fs/test.zig#L156-L184
func Test_fdReaddir_opened_file_written(t *testing.T) {
	root := t.TempDir()
	testFS := NewDirFS(root)

	const readDirTarget = "dir"
	err := testFS.Mkdir(readDirTarget, 0o700)
	require.NoError(t, err)

	// Open the directory, before writing files!
	dirFile, err := testFS.OpenFile(readDirTarget, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer dirFile.Close()

	// Then write a file to the directory.
	f, err := os.Create(pathutil.Join(root, readDirTarget, "my-file"))
	require.NoError(t, err)
	defer f.Close()

	dir, ok := dirFile.(fs.ReadDirFile)
	require.True(t, ok)

	entries, err := dir.ReadDir(-1)
	require.NoError(t, err)

	require.Equal(t, 1, len(entries))
	require.Equal(t, "my-file", entries[0].Name())
}

func TestDirFS_Link(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	require.ErrorIs(t, testFS.Link("cat", ""), syscall.ENOENT)
	require.ErrorIs(t, testFS.Link("sub/test.txt", "sub/test.txt"), syscall.EEXIST)
	require.ErrorIs(t, testFS.Link("sub/test.txt", "."), syscall.EEXIST)
	require.ErrorIs(t, testFS.Link("sub/test.txt", ""), syscall.EEXIST)
	require.ErrorIs(t, testFS.Link("sub/test.txt", "/"), syscall.EEXIST)
	require.NoError(t, testFS.Link("sub/test.txt", "foo"))
}

func TestDirFS_Symlink(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	require.ErrorIs(t, testFS.Symlink("sub/test.txt", "sub/test.txt"), syscall.EEXIST)
	// Non-existing old name is allowed.
	require.NoError(t, testFS.Symlink("non-existing", "aa"))
	require.NoError(t, testFS.Symlink("sub/", "symlinked-subdir"))

	st, err := os.Lstat(pathutil.Join(tmpDir, "aa"))
	require.NoError(t, err)
	require.Equal(t, "aa", st.Name())
	require.True(t, st.Mode()&fs.ModeSymlink > 0 && !st.IsDir())

	st, err = os.Lstat(pathutil.Join(tmpDir, "symlinked-subdir"))
	require.NoError(t, err)
	require.Equal(t, "symlinked-subdir", st.Name())
	require.True(t, st.Mode()&fs.ModeSymlink > 0)
}

func TestDirFS_Readlink(t *testing.T) {
	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	testLinks := []struct {
		old, dst string
	}{
		// Same dir.
		{old: "animals.txt", dst: "symlinked-animals.txt"},
		{old: "sub/test.txt", dst: "sub/symlinked-test.txt"},
		// Parent to sub.
		{old: "animals.txt", dst: "sub/symlinked-animals.txt"},
		// Sub to parent.
		{old: "sub/test.txt", dst: "symlinked-zoo.txt"},
	}

	buf := make([]byte, 200)
	for _, tl := range testLinks {
		err := testFS.Symlink(tl.old, tl.dst) // not os.Symlink for windows compat
		require.NoError(t, err, "%v", tl)

		n, err := testFS.Readlink(tl.dst, buf)
		require.NoError(t, err)
		require.Equal(t, tl.old, string(buf[:n]))
		require.Equal(t, len(tl.old), n)
	}

	t.Run("errors", func(t *testing.T) {
		_, err := testFS.Readlink("sub/test.txt", buf)
		require.Error(t, err)
		_, err = testFS.Readlink("", buf)
		require.Error(t, err)
		_, err = testFS.Readlink("animals.txt", buf)
		require.Error(t, err)
	})
}
