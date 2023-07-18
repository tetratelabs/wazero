package sysfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewDirFS(t *testing.T) {
	testFS := NewDirFS(".")

	// Guest can look up /
	f, errno := testFS.OpenFile("/", fsapi.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	require.EqualErrno(t, 0, f.Close())

	t.Run("host path not found", func(t *testing.T) {
		testFS := NewDirFS("a")
		_, errno = testFS.OpenFile(".", fsapi.O_RDONLY, 0)
		require.EqualErrno(t, sys.ENOENT, errno)
	})
	t.Run("host path not a directory", func(t *testing.T) {
		arg0 := os.Args[0] // should be safe in scratch tests which don't have the source mounted.

		testFS := NewDirFS(arg0)
		d, errno := testFS.OpenFile(".", fsapi.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		_, errno = d.Readdir(-1)
		require.EqualErrno(t, sys.EBADF, errno)
	})
}

func TestDirFS_join(t *testing.T) {
	testFS := NewDirFS("/").(*dirFS)
	require.Equal(t, "/", testFS.join(""))
	require.Equal(t, "/", testFS.join("."))
	require.Equal(t, "/", testFS.join("/"))
	require.Equal(t, "/tmp", testFS.join("tmp"))

	testFS = NewDirFS(".").(*dirFS)
	require.Equal(t, ".", testFS.join(""))
	require.Equal(t, ".", testFS.join("."))
	require.Equal(t, ".", testFS.join("/"))
	require.Equal(t, "."+string(os.PathSeparator)+"tmp", testFS.join("tmp"))
}

func TestDirFS_String(t *testing.T) {
	testFS := NewDirFS(".")

	// String has the name of the path entered
	require.Equal(t, ".", testFS.(fmt.Stringer).String())
}

func TestDirFS_Lstat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)
	for _, path := range []string{"animals.txt", "sub", "sub-link"} {
		require.EqualErrno(t, 0, testFS.Symlink(path, path+"-link"))
	}

	testLstat(t, testFS)
}

func TestDirFS_MkDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	name := "mkdir"
	realPath := path.Join(tmpDir, name)

	t.Run("doesn't exist", func(t *testing.T) {
		require.EqualErrno(t, 0, testFS.Mkdir(name, fs.ModeDir))

		stat, err := os.Stat(realPath)
		require.NoError(t, err)

		require.Equal(t, name, stat.Name())
		require.True(t, stat.IsDir())
	})

	t.Run("dir exists", func(t *testing.T) {
		err := testFS.Mkdir(name, fs.ModeDir)
		require.EqualErrno(t, sys.EEXIST, err)
	})

	t.Run("file exists", func(t *testing.T) {
		require.NoError(t, os.Remove(realPath))
		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Mkdir(name, fs.ModeDir)
		require.EqualErrno(t, sys.EEXIST, err)
	})
	t.Run("try creating on file", func(t *testing.T) {
		filePath := path.Join("non-existing-dir", "foo.txt")
		err := testFS.Mkdir(filePath, fs.ModeDir)
		require.EqualErrno(t, sys.ENOENT, err)
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

func testChmod(t *testing.T, testFS fsapi.FS, path string) {
	// Test base case, using 0o444 not 0o400 for read-back on windows.
	requireMode(t, testFS, path, 0o444)

	// Test adding write, using 0o666 not 0o600 for read-back on windows.
	require.EqualErrno(t, 0, testFS.Chmod(path, 0o666))
	requireMode(t, testFS, path, 0o666)

	if runtime.GOOS != "windows" {
		// Test clearing group and world, setting owner read+execute.
		require.EqualErrno(t, 0, testFS.Chmod(path, 0o500))
		requireMode(t, testFS, path, 0o500)
	}
}

func requireMode(t *testing.T, testFS fsapi.FS, path string, mode fs.FileMode) {
	st, errno := testFS.Stat(path)
	require.EqualErrno(t, 0, errno)

	require.Equal(t, mode, st.Mode.Perm())
}

func TestDirFS_Rename(t *testing.T) {
	t.Run("from doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		err := os.WriteFile(file1Path, []byte{1}, 0o600)
		require.NoError(t, err)

		err = testFS.Rename("file2", file1)
		require.EqualErrno(t, sys.ENOENT, err)
	})
	t.Run("file to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		errno := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, errno)

		file2 := "file2"
		file2Path := path.Join(tmpDir, file2)
		errno = testFS.Rename(file1, file2)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, errno = os.Stat(file1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(errno))

		s, err := os.Stat(file2Path)
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})
	t.Run("dir to non-exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)
		errrno := testFS.Rename(dir1, dir2)
		require.EqualErrno(t, 0, errrno)

		// Show the prior path no longer exists
		_, err := os.Stat(dir1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		s, err := os.Stat(dir2Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("dir to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)

		// write a file to that path
		f, err := os.OpenFile(dir2Path, os.O_RDWR|os.O_CREATE, 0o600)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		errno := testFS.Rename(dir1, dir2)
		require.EqualErrno(t, sys.ENOTDIR, errno)
	})
	t.Run("file to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		errno := testFS.Rename(file1, dir1)
		require.EqualErrno(t, sys.EISDIR, errno)
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L567-L582
	t.Run("dir to empty dir should be fine", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := path.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		errno := testFS.Rename(dir1, dir2)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err = os.Stat(dir1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		// Show the file inside that directory moved
		s, err := os.Stat(path.Join(dir2Path, file1))
		require.NoError(t, err)
		require.False(t, s.IsDir())
	})

	// Similar to https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs/test.zig#L584-L604
	t.Run("dir to non empty dir should be EXIST", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		// add a file to that directory
		file1 := "file1"
		file1Path := path.Join(dir1Path, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		dir2 := "dir2"
		dir2Path := path.Join(tmpDir, dir2)
		require.NoError(t, os.Mkdir(dir2Path, 0o700))

		// Make the destination non-empty.
		err = os.WriteFile(path.Join(dir2Path, "existing.txt"), []byte("any thing"), 0o600)
		require.NoError(t, err)

		errno := testFS.Rename(dir1, dir2)
		require.EqualErrno(t, sys.ENOTEMPTY, errno)
	})

	t.Run("file to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

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

		errno := testFS.Rename(file1, file2)
		require.EqualErrno(t, 0, errno)

		// Show the prior path no longer exists
		_, err = os.Stat(file1Path)
		require.EqualErrno(t, sys.ENOENT, sys.UnwrapOSError(err))

		// Show the file1 overwrote file2
		b, err := os.ReadFile(file2Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
	t.Run("dir to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir1 := "dir1"
		dir1Path := path.Join(tmpDir, dir1)
		require.NoError(t, os.Mkdir(dir1Path, 0o700))

		errno := testFS.Rename(dir1, dir1)
		require.EqualErrno(t, 0, errno)

		s, err := os.Stat(dir1Path)
		require.NoError(t, err)
		require.True(t, s.IsDir())
	})
	t.Run("file to itself", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		file1 := "file1"
		file1Path := path.Join(tmpDir, file1)
		file1Contents := []byte{1}
		err := os.WriteFile(file1Path, file1Contents, 0o600)
		require.NoError(t, err)

		errno := testFS.Rename(file1, file1)
		require.EqualErrno(t, 0, errno)

		b, err := os.ReadFile(file1Path)
		require.NoError(t, err)
		require.Equal(t, file1Contents, b)
	})
}

func TestDirFS_Rmdir(t *testing.T) {
	t.Run("doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"

		err := testFS.Rmdir(name)
		require.EqualErrno(t, sys.ENOENT, err)
	})

	t.Run("dir not empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"
		realPath := path.Join(tmpDir, name)

		require.NoError(t, os.Mkdir(realPath, 0o700))
		fileInDir := path.Join(realPath, "file")
		require.NoError(t, os.WriteFile(fileInDir, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.EqualErrno(t, sys.ENOTEMPTY, err)

		require.NoError(t, os.Remove(fileInDir))
	})

	t.Run("dir previously not empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"
		realPath := path.Join(tmpDir, name)
		require.NoError(t, os.Mkdir(realPath, 0o700))

		// Create a file and then delete it.
		fileInDir := path.Join(realPath, "file")
		require.NoError(t, os.WriteFile(fileInDir, []byte{}, 0o600))
		require.NoError(t, os.Remove(fileInDir))

		// After deletion, try removing directory.
		errno := testFS.Rmdir(name)
		require.EqualErrno(t, 0, errno)
	})

	t.Run("dir empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"
		realPath := path.Join(tmpDir, name)
		require.NoError(t, os.Mkdir(realPath, 0o700))
		require.EqualErrno(t, 0, testFS.Rmdir(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})

	t.Run("dir empty while opening", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"
		realPath := path.Join(tmpDir, name)
		require.NoError(t, os.Mkdir(realPath, 0o700))

		f, errno := testFS.OpenFile(name, fsapi.O_DIRECTORY, 0o700)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		require.EqualErrno(t, 0, testFS.Rmdir(name))
		_, err := os.Stat(realPath)
		require.Error(t, err)
	})

	t.Run("not directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "rmdir"
		realPath := path.Join(tmpDir, name)

		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		err := testFS.Rmdir(name)
		require.EqualErrno(t, sys.ENOTDIR, err)

		require.NoError(t, os.Remove(realPath))
	})
}

func TestDirFS_Unlink(t *testing.T) {
	t.Run("doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)
		name := "unlink"

		err := testFS.Unlink(name)
		require.EqualErrno(t, sys.ENOENT, err)
	})

	t.Run("target: dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		dir := "dir"
		realPath := path.Join(tmpDir, dir)

		require.NoError(t, os.Mkdir(realPath, 0o700))

		err := testFS.Unlink(dir)
		require.EqualErrno(t, sys.EISDIR, err)

		require.NoError(t, os.Remove(realPath))
	})

	t.Run("target: symlink to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		// Create link target dir.
		subDirName := "subdir"
		subDirRealPath := path.Join(tmpDir, subDirName)
		require.NoError(t, os.Mkdir(subDirRealPath, 0o700))

		// Create a symlink to the subdirectory.
		const symlinkName = "symlink-to-dir"
		require.EqualErrno(t, 0, testFS.Symlink("subdir", symlinkName))

		// Unlinking the symlink should suceed.
		errno := testFS.Unlink(symlinkName)
		require.EqualErrno(t, 0, errno)
	})

	t.Run("file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "unlink"
		realPath := path.Join(tmpDir, name)

		require.NoError(t, os.WriteFile(realPath, []byte{}, 0o600))

		require.EqualErrno(t, 0, testFS.Unlink(name))

		_, err := os.Stat(realPath)
		require.Error(t, err)
	})
}

func TestDirFS_Utimesns(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	file := "file"
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	t.Run("doesn't exist", func(t *testing.T) {
		err := testFS.Utimens("nope", nil, true)
		require.EqualErrno(t, sys.ENOENT, err)
		err = testFS.Utimens("nope", nil, false)
		if SupportsSymlinkNoFollow {
			require.EqualErrno(t, sys.ENOENT, err)
		} else {
			require.EqualErrno(t, sys.ENOSYS, err)
		}
	})

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	//
	// Negative isn't tested as most platforms don't return consistent results.
	tests := []struct {
		name  string
		times *[2]syscall.Timespec
	}{
		{
			name: "nil",
		},
		{
			name: "a=omit,m=omit",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_OMIT},
				{Sec: 123, Nsec: UTIME_OMIT},
			},
		},
		{
			name: "a=now,m=omit",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: UTIME_OMIT},
			},
		},
		{
			name: "a=omit,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_OMIT},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=now,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=now,m=set",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: 4 * 1e3},
			},
		},
		{
			name: "a=set,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: 4 * 1e3},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=set,m=set",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: 4 * 1e3},
				{Sec: 223, Nsec: 5 * 1e3},
			},
		},
	}

	for _, fileType := range []string{"dir", "file", "link", "link-follow"} {
		for _, tt := range tests {
			tc := tt
			fileType := fileType
			name := fileType + " " + tc.name
			symlinkNoFollow := fileType == "link"

			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				testFS := NewDirFS(tmpDir)

				file := path.Join(tmpDir, "file")
				errno := os.WriteFile(file, []byte{}, 0o700)
				require.NoError(t, errno)

				link := file + "-link"
				require.NoError(t, os.Symlink(file, link))

				dir := path.Join(tmpDir, "dir")
				errno = os.Mkdir(dir, 0o700)
				require.NoError(t, errno)

				var path, statPath string
				switch fileType {
				case "dir":
					path = "dir"
					statPath = "dir"
				case "file":
					path = "file"
					statPath = "file"
				case "link":
					path = "file-link"
					statPath = "file-link"
				case "link-follow":
					path = "file-link"
					statPath = "file"
				default:
					panic(tc)
				}

				oldSt, errno := testFS.Lstat(statPath)
				require.EqualErrno(t, 0, errno)

				errno = testFS.Utimens(path, tc.times, !symlinkNoFollow)
				if symlinkNoFollow && !SupportsSymlinkNoFollow {
					require.EqualErrno(t, sys.ENOSYS, errno)
					return
				}
				require.EqualErrno(t, 0, errno)

				newSt, errno := testFS.Lstat(statPath)
				require.EqualErrno(t, 0, errno)

				if platform.CompilerSupported() {
					if tc.times != nil && tc.times[0].Nsec == UTIME_OMIT {
						require.Equal(t, oldSt.Atim, newSt.Atim)
					} else if tc.times == nil || tc.times[0].Nsec == UTIME_NOW {
						now := time.Now().UnixNano()
						require.True(t, newSt.Atim <= now, "expected atim %d <= now %d", newSt.Atim, now)
					} else {
						require.Equal(t, tc.times[0].Nano(), newSt.Atim)
					}
				}

				// When compiler isn't supported, we can still check mtim.
				if tc.times != nil && tc.times[1].Nsec == UTIME_OMIT {
					require.Equal(t, oldSt.Mtim, newSt.Mtim)
				} else if tc.times == nil || tc.times[1].Nsec == UTIME_NOW {
					now := time.Now().UnixNano()
					require.True(t, newSt.Mtim <= now, "expected mtim %d <= now %d", newSt.Mtim, now)
				} else {
					require.Equal(t, tc.times[1].Nano(), newSt.Mtim)
				}
			})
		}
	}
}

func TestDirFS_OpenFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the fsapi.FS root.
	tmpDir = path.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	testOpen_Read(t, testFS, statSetsIno(), true)

	testOpen_O_RDWR(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", fsapi.O_RDONLY, 0)

		// fsapi.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestDirFS_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)
	testStat(t, testFS)

	// from os.TestDirFSPathsValid
	if runtime.GOOS != "windows" {
		t.Run("strange name", func(t *testing.T) {
			name := `e:xperi\ment.txt`
			require.NoError(t, os.WriteFile(path.Join(tmpDir, name), nil, 0o600))

			_, errno := testFS.Stat(name)
			require.EqualErrno(t, 0, errno)
		})
	}
}

func TestDirFS_Readdir(t *testing.T) {
	root := t.TempDir()
	testFS := NewDirFS(root)

	const readDirTarget = "dir"
	errno := testFS.Mkdir(readDirTarget, 0o700)
	require.EqualErrno(t, 0, errno)

	// Open the empty directory
	dirFile, errno := testFS.OpenFile(readDirTarget, fsapi.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	defer dirFile.Close()

	// Write files to the directory after it is open.
	require.NoError(t, os.WriteFile(path.Join(root, readDirTarget, "1"), nil, 0o444))
	require.NoError(t, os.WriteFile(path.Join(root, readDirTarget, "2"), nil, 0o444))

	// Test files are visible. This fails in windows unless the implementation
	// re-opens the underlying file.
	// https://github.com/ziglang/zig/blob/e3736baddb8ecff90f0594be9f604c7484ce9aa2/lib/std/fs/test.zig#L290-L317
	t.Run("Sees files written after open", func(t *testing.T) {
		dirents, errno := dirFile.Readdir(1)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, 1, len(dirents))
		n := dirents[0].Name
		switch n {
		case "1", "2": // order is inconsistent on scratch images.
		default:
			require.Equal(t, "1", n)
		}
	})

	// Test there is no error reading the directory if it was deleted while
	// iterating. See docs on Readdir for why in general, but specifically Zig
	// tests enforce this. This test is Windows sensitive as well.
	//
	// https://github.com/ziglang/zig/blob/e3736baddb8ecff90f0594be9f604c7484ce9aa2/lib/std/fs/test.zig#L311C1-L311C1
	t.Run("sys.ENOENT or no error, deleted while reading", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(path.Join(root, readDirTarget)))

		dirents, errno := dirFile.Readdir(-1)
		if errno != 0 {
			require.EqualErrno(t, sys.ENOENT, errno)
		}

		require.Equal(t, 0, len(dirents))
	})
}

func TestDirFS_Link(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	require.EqualErrno(t, testFS.Link("cat", ""), sys.ENOENT)
	require.EqualErrno(t, testFS.Link("sub/test.txt", "sub/test.txt"), sys.EEXIST)
	require.EqualErrno(t, testFS.Link("sub/test.txt", "."), sys.EEXIST)
	require.EqualErrno(t, testFS.Link("sub/test.txt", ""), sys.EEXIST)
	require.EqualErrno(t, testFS.Link("sub/test.txt", "/"), sys.EEXIST)
	require.EqualErrno(t, 0, testFS.Link("sub/test.txt", "foo"))
}

func TestDirFS_Symlink(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)

	require.EqualErrno(t, sys.EEXIST, testFS.Symlink("sub/test.txt", "sub/test.txt"))
	// Non-existing old name is allowed.
	require.EqualErrno(t, 0, testFS.Symlink("non-existing", "aa"))
	require.EqualErrno(t, 0, testFS.Symlink("sub/", "symlinked-subdir"))

	st, err := os.Lstat(path.Join(tmpDir, "aa"))
	require.NoError(t, err)
	require.Equal(t, "aa", st.Name())
	require.True(t, st.Mode()&fs.ModeSymlink > 0 && !st.IsDir())

	st, err = os.Lstat(path.Join(tmpDir, "symlinked-subdir"))
	require.NoError(t, err)
	require.Equal(t, "symlinked-subdir", st.Name())
	require.True(t, st.Mode()&fs.ModeSymlink > 0)
}

func TestDirFS_Readlink(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testFS := NewDirFS(tmpDir)
	testReadlink(t, testFS, testFS)
}
