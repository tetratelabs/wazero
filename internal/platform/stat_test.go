package platform

import (
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestLstat(t *testing.T) {
	tmpDir := t.TempDir()

	_, errno := Lstat(path.Join(tmpDir, "cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = Lstat(path.Join(tmpDir, "sub/cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)

	var st Stat_t
	t.Run("dir", func(t *testing.T) {
		st, errno = Lstat(tmpDir)
		require.Zero(t, errno)

		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	file := path.Join(tmpDir, "file")
	var stFile Stat_t

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(file, []byte{1, 2}, 0o400))
		stFile, errno = Lstat(file)
		require.Zero(t, errno)

		require.Zero(t, stFile.Mode.Type())
		require.Equal(t, int64(2), stFile.Size)
		require.NotEqual(t, uint64(0), stFile.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		requireLinkStat(t, file, stFile)
	})

	subdir := path.Join(tmpDir, "sub")
	var stSubdir Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		stSubdir, errno = Lstat(subdir)
		require.Zero(t, errno)

		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), stSubdir.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		requireLinkStat(t, subdir, stSubdir)
	})

	t.Run("link to dir link", func(t *testing.T) {
		pathLink := subdir + "-link"
		stLink, errno := Lstat(pathLink)
		require.Zero(t, errno)

		requireLinkStat(t, pathLink, stLink)
	})
}

func requireLinkStat(t *testing.T, path string, stat Stat_t) {
	link := path + "-link"
	require.NoError(t, os.Symlink(path, link))

	stLink, errno := Lstat(link)
	require.Zero(t, errno)

	require.NotEqual(t, uint64(0), stLink.Ino)
	require.NotEqual(t, stat.Ino, stLink.Ino) // inodes are not equal
	require.Equal(t, fs.ModeSymlink, stLink.Mode.Type())
	// From https://linux.die.net/man/2/lstat:
	// The size of a symbolic link is the length of the pathname it
	// contains, without a terminating null byte.
	if runtime.GOOS == "windows" { // size is zero, not the path length
		require.Zero(t, stLink.Size)
	} else {
		require.Equal(t, int64(len(path)), stLink.Size)
	}
}

func TestStat(t *testing.T) {
	tmpDir := t.TempDir()

	_, errno := Stat(path.Join(tmpDir, "cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = Stat(path.Join(tmpDir, "sub/cat"))
	require.EqualErrno(t, syscall.ENOENT, errno)

	var st Stat_t

	t.Run("dir", func(t *testing.T) {
		st, errno = Stat(tmpDir)
		require.Zero(t, errno)

		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	file := path.Join(tmpDir, "file")
	var stFile Stat_t

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(file, nil, 0o400))

		stFile, errno = Stat(file)
		require.Zero(t, errno)

		require.False(t, stFile.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		link := path.Join(tmpDir, "file-link")
		require.NoError(t, os.Symlink(file, link))

		stLink, errno := Stat(link)
		require.Zero(t, errno)

		require.Equal(t, stFile, stLink) // resolves to the file
	})

	subdir := path.Join(tmpDir, "sub")
	var stSubdir Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		stSubdir, errno = Stat(subdir)
		require.Zero(t, errno)

		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		link := path.Join(tmpDir, "dir-link")
		require.NoError(t, os.Symlink(subdir, link))

		stLink, errno := Stat(link)
		require.Zero(t, errno)

		require.Equal(t, stSubdir, stLink) // resolves to the dir
	})
}

func TestStatFile(t *testing.T) {
	tmpDir := t.TempDir()

	var st Stat_t

	tmpDirF, errno := OpenFile(tmpDir, syscall.O_RDONLY, 0)
	require.Zero(t, errno)
	defer tmpDirF.Close()

	t.Run("dir", func(t *testing.T) {
		st, errno = StatFile(tmpDirF)
		require.Zero(t, errno)

		require.True(t, st.Mode.IsDir())
		requireDirectoryDevIno(t, st)
	})

	// Windows allows you to stat a closed dir because it is accessed by path,
	// not by file descriptor.
	if runtime.GOOS != "windows" {
		t.Run("closed dir", func(t *testing.T) {
			require.NoError(t, tmpDirF.Close())
			st, errno = StatFile(tmpDirF)
			require.EqualErrno(t, syscall.EBADF, errno)
		})
	}

	file := path.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o400))
	fileF, errno := OpenFile(file, syscall.O_RDONLY, 0)
	require.Zero(t, errno)
	defer fileF.Close()

	t.Run("file", func(t *testing.T) {
		st, errno = StatFile(fileF)
		require.Zero(t, errno)

		require.False(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("closed file", func(t *testing.T) {
		require.NoError(t, fileF.Close())
		_, errno = StatFile(fileF)
		require.EqualErrno(t, syscall.EBADF, errno)
	})

	subdir := path.Join(tmpDir, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o500))
	subdirF, errno := OpenFile(subdir, syscall.O_RDONLY, 0)
	require.Zero(t, errno)
	defer subdirF.Close()

	t.Run("subdir", func(t *testing.T) {
		st, errno = StatFile(subdirF)
		require.Zero(t, errno)

		require.True(t, st.Mode.IsDir())
		requireDirectoryDevIno(t, st)
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed subdir", func(t *testing.T) {
			require.NoError(t, subdirF.Close())
			st, errno = StatFile(subdirF)
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

			f, err := os.Open(file)
			require.NoError(t, err)
			defer f.Close()

			st, errno := StatFile(f)
			require.Zero(t, errno)

			require.Equal(t, st.Atim, tc.atimeNsec)
			require.Equal(t, st.Mtim, tc.mtimeNsec)
		})
	}
}

func TestStatFile_dev_inode(t *testing.T) {
	tmpDir := t.TempDir()
	d, err := os.Open(tmpDir)
	require.NoError(t, err)
	defer d.Close()

	path1 := path.Join(tmpDir, "1")
	f1, err := os.Create(path1)
	require.NoError(t, err)
	defer f1.Close()

	path2 := path.Join(tmpDir, "2")
	f2, err := os.Create(path2)
	require.NoError(t, err)
	defer f2.Close()

	pathLink2 := path.Join(tmpDir, "link2")
	err = os.Symlink(path2, pathLink2)
	require.NoError(t, err)
	l2, err := os.Open(pathLink2)
	require.NoError(t, err)
	defer l2.Close()

	// First, stat the directory
	st1, errno := StatFile(d)
	require.Zero(t, errno)

	requireDirectoryDevIno(t, st1)

	// Now, stat the files in it
	st1, errno = StatFile(f1)
	require.Zero(t, errno)

	st2, errno := StatFile(f2)
	require.Zero(t, errno)

	st3, errno := StatFile(l2)
	require.Zero(t, errno)

	// The files should be on the same device, but different inodes
	require.Equal(t, st1.Dev, st2.Dev)
	require.NotEqual(t, st1.Ino, st2.Ino)
	require.Equal(t, st2, st3) // stat on a link is for its target

	// Redoing stat should result in the same inodes
	st1Again, errno := StatFile(f1)
	require.Zero(t, errno)
	require.Equal(t, st1.Dev, st1Again.Dev)

	// On Windows, we cannot rename while opening.
	// So we manually close here before renaming.
	require.NoError(t, f1.Close())
	require.NoError(t, f2.Close())
	require.NoError(t, l2.Close())

	// Renaming a file shouldn't change its inodes.
	require.Zero(t, Rename(path1, path2))
	f1, err = os.Open(path2)
	require.NoError(t, err)
	defer f1.Close()

	st1Again, errno = StatFile(f1)
	require.Zero(t, errno)
	require.Equal(t, st1.Dev, st1Again.Dev)
	require.Equal(t, st1.Ino, st1Again.Ino)
}

func requireDirectoryDevIno(t *testing.T, st Stat_t) {
	// windows before go 1.20 has trouble reading the inode information on
	// directories.
	if runtime.GOOS != "windows" || IsGo120 {
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
		require.NoError(t, os.Mkdir(dir, 0o0700))
		require.Zero(t, chgid(dir, gid))

		st, errno := Stat(dir)
		require.Zero(t, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})

	t.Run("LStat", func(t *testing.T) {
		tmpDir := t.TempDir()
		link := path.Join(tmpDir, "link")
		require.NoError(t, os.Symlink(tmpDir, link))
		require.Zero(t, chgid(link, gid))

		st, errno := Lstat(link)
		require.Zero(t, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})

	t.Run("StatFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		file := path.Join(tmpDir, "file")
		require.NoError(t, os.WriteFile(file, nil, 0o0600))
		require.Zero(t, chgid(file, gid))

		st, errno := Lstat(file)
		require.Zero(t, errno)

		require.Equal(t, uid, st.Uid)
		require.Equal(t, gid, st.Gid)
	})
}

func chgid(path string, gid uint32) error {
	// Note: In Chown, -1 is means leave the uid alone
	return Chown(path, -1, int(gid))
}
