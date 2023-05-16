package sysfs

import (
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStat(t *testing.T) {
	tmpDir := t.TempDir()

	_, errno := stat(path.Join(tmpDir, "cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = stat(path.Join(tmpDir, "sub/cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)

	var st fsapi.Stat_t

	t.Run("dir", func(t *testing.T) {
		st, errno = stat(tmpDir)
		require.EqualErrno(t, 0, errno)

		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	file := path.Join(tmpDir, "file")
	var stFile fsapi.Stat_t

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(file, nil, 0o400))

		stFile, errno = stat(file)
		require.EqualErrno(t, 0, errno)

		require.False(t, stFile.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		link := path.Join(tmpDir, "file-link")
		require.NoError(t, os.Symlink(file, link))

		stLink, errno := stat(link)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, stFile, stLink) // resolves to the file
	})

	subdir := path.Join(tmpDir, "sub")
	var stSubdir fsapi.Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		stSubdir, errno = stat(subdir)
		require.EqualErrno(t, 0, errno)

		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		link := path.Join(tmpDir, "dir-link")
		require.NoError(t, os.Symlink(subdir, link))

		stLink, errno := stat(link)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, stSubdir, stLink) // resolves to the dir
	})
}

func TestStatFile(t *testing.T) {
	tmpDir := t.TempDir()

	tmpDirF := requireOpenFile(t, tmpDir, syscall.O_RDONLY, 0)
	defer tmpDirF.Close()

	t.Run("dir", func(t *testing.T) {
		st, errno := tmpDirF.Stat()
		require.EqualErrno(t, 0, errno)

		require.True(t, st.Mode.IsDir())
		requireDirectoryDevIno(t, st)
	})

	// Windows allows you to stat a closed dir because it is accessed by path,
	// not by file descriptor.
	if runtime.GOOS != "windows" {
		t.Run("closed dir", func(t *testing.T) {
			require.EqualErrno(t, 0, tmpDirF.Close())
			_, errno := tmpDirF.Stat()
			require.EqualErrno(t, syscall.EBADF, errno)
		})
	}

	file := path.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o400))
	fileF := requireOpenFile(t, file, syscall.O_RDONLY, 0)
	defer fileF.Close()

	t.Run("file", func(t *testing.T) {
		st, errno := fileF.Stat()
		require.EqualErrno(t, 0, errno)

		require.False(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("closed fsFile", func(t *testing.T) {
		require.EqualErrno(t, 0, fileF.Close())
		_, errno := fileF.Stat()
		require.EqualErrno(t, syscall.EBADF, errno)
	})

	subdir := path.Join(tmpDir, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o500))
	subdirF := requireOpenFile(t, subdir, syscall.O_RDONLY, 0)
	defer subdirF.Close()

	t.Run("subdir", func(t *testing.T) {
		st, errno := subdirF.Stat()
		require.EqualErrno(t, 0, errno)

		require.True(t, st.Mode.IsDir())
		requireDirectoryDevIno(t, st)
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed subdir", func(t *testing.T) {
			require.EqualErrno(t, 0, subdirF.Close())
			_, errno := subdirF.Stat()
			require.EqualErrno(t, syscall.EBADF, errno)
		})
	}
}

func Test_StatFile_times(t *testing.T) {
	tmpDir := t.TempDir()

	file := path.Join(tmpDir, "file")
	err := os.WriteFile(file, []byte{}, 0o700)
	require.NoError(t, err)

	type test struct {
		name                 string
		atimeNsec, mtimeNsec int64
	}
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{
			name:      "positive",
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "zero"},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{
				name:      "negative",
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := os.Chtimes(file, time.UnixMicro(tc.atimeNsec/1e3), time.UnixMicro(tc.mtimeNsec/1e3))
			require.NoError(t, err)

			f := requireOpenFile(t, file, syscall.O_RDONLY, 0)
			defer f.Close()

			st, errno := f.Stat()
			require.EqualErrno(t, 0, errno)

			require.Equal(t, st.Atim, tc.atimeNsec)
			require.Equal(t, st.Mtim, tc.mtimeNsec)
		})
	}
}

func TestStatFile_dev_inode(t *testing.T) {
	tmpDir := t.TempDir()
	d := requireOpenFile(t, tmpDir, os.O_RDONLY, 0)
	defer d.Close()

	path1 := path.Join(tmpDir, "1")
	f1 := requireOpenFile(t, path1, os.O_CREATE, 0o666)
	defer f1.Close()

	path2 := path.Join(tmpDir, "2")
	f2 := requireOpenFile(t, path2, os.O_CREATE, 0o666)
	defer f2.Close()

	pathLink2 := path.Join(tmpDir, "link2")
	err := os.Symlink(path2, pathLink2)
	require.NoError(t, err)
	l2 := requireOpenFile(t, pathLink2, os.O_RDONLY, 0)
	defer l2.Close()

	// First, stat the directory
	st1, errno := d.Stat()
	require.EqualErrno(t, 0, errno)

	requireDirectoryDevIno(t, st1)

	// Now, stat the files in it
	st1, errno = f1.Stat()
	require.EqualErrno(t, 0, errno)

	st2, errno := f2.Stat()
	require.EqualErrno(t, 0, errno)

	st3, errno := l2.Stat()
	require.EqualErrno(t, 0, errno)

	// The files should be on the same device, but different inodes
	require.Equal(t, st1.Dev, st2.Dev)
	require.NotEqual(t, st1.Ino, st2.Ino)
	require.Equal(t, st2, st3) // stat on a link is for its target

	// Redoing stat should result in the same inodes
	st1Again, errno := f1.Stat()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, st1.Dev, st1Again.Dev)

	// On Windows, we cannot rename while opening.
	// So we manually close here before renaming.
	require.EqualErrno(t, 0, f1.Close())
	require.EqualErrno(t, 0, f2.Close())
	require.EqualErrno(t, 0, l2.Close())

	// Renaming a file shouldn't change its inodes.
	require.EqualErrno(t, 0, Rename(path1, path2))
	f1 = requireOpenFile(t, path2, os.O_RDONLY, 0)
	defer f1.Close()

	st1Again, errno = f1.Stat()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, st1.Dev, st1Again.Dev)
	require.Equal(t, st1.Ino, st1Again.Ino)
}

func requireDirectoryDevIno(t *testing.T, st fsapi.Stat_t) {
	// windows before go 1.20 has trouble reading the inode information on
	// directories.
	if runtime.GOOS != "windows" || platform.IsGo120 {
		require.NotEqual(t, uint64(0), st.Dev)
		require.NotEqual(t, uint64(0), st.Ino)
	} else {
		require.Zero(t, st.Dev)
		require.Zero(t, st.Ino)
	}
}

// TestStat_uid_gid is similar to os.TestChown
func TestStat_uid_gid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows")
	}

	// We don't attempt changing the uid of a file, as only root can do that.
	// Also, this isn't a test of chown. The main goal here is to read-back
	// the uid, gid, both of which are zero if run as root.
	uid := uint32(os.Getuid())
	gid := uint32(os.Getgid())

	t.Run("Stat", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := path.Join(tmpDir, "dir")
		require.NoError(t, os.Mkdir(dir, 0o0777))
		require.EqualErrno(t, 0, chgid(dir, gid))

		st, errno := stat(dir)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})

	t.Run("LStat", func(t *testing.T) {
		tmpDir := t.TempDir()
		link := path.Join(tmpDir, "link")
		require.NoError(t, os.Symlink(tmpDir, link))
		require.EqualErrno(t, 0, chgid(link, gid))

		st, errno := lstat(link)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})

	t.Run("statFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		file := path.Join(tmpDir, "file")
		require.NoError(t, os.WriteFile(file, nil, 0o0666))
		require.EqualErrno(t, 0, chgid(file, gid))

		st, errno := lstat(file)
		require.EqualErrno(t, 0, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})
}

func chgid(path string, gid uint32) error {
	// Note: In Chown, -1 is means leave the uid alone
	return Chown(path, -1, int(gid))
}
