package platform

import (
	"io/fs"
	"os"
	pathutil "path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestLstat(t *testing.T) {
	tmpDir := t.TempDir()

	var stat Stat_t
	require.EqualErrno(t, syscall.ENOENT, Lstat(pathutil.Join(tmpDir, "cat"), &stat))
	require.EqualErrno(t, syscall.ENOENT, Lstat(pathutil.Join(tmpDir, "sub/cat"), &stat))

	t.Run("dir", func(t *testing.T) {
		err := Lstat(tmpDir, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	file := pathutil.Join(tmpDir, "file")
	var statFile Stat_t

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(file, []byte{1, 2}, 0o400))
		require.NoError(t, Lstat(file, &statFile))
		require.Zero(t, statFile.Mode.Type())
		require.Equal(t, int64(2), statFile.Size)
		require.NotEqual(t, uint64(0), statFile.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		requireLinkStat(t, file, &statFile)
	})

	subdir := pathutil.Join(tmpDir, "sub")
	var statSubdir Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		require.NoError(t, Lstat(subdir, &statSubdir))
		require.True(t, statSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), statSubdir.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		requireLinkStat(t, subdir, &statSubdir)
	})

	t.Run("link to dir link", func(t *testing.T) {
		pathLink := subdir + "-link"
		var statLink Stat_t
		require.NoError(t, Lstat(pathLink, &statLink))

		requireLinkStat(t, pathLink, &statLink)
	})
}

func requireLinkStat(t *testing.T, path string, stat *Stat_t) {
	link := path + "-link"
	var linkStat Stat_t
	require.NoError(t, os.Symlink(path, link))

	require.NoError(t, Lstat(link, &linkStat))
	require.NotEqual(t, uint64(0), linkStat.Ino)
	require.NotEqual(t, stat.Ino, linkStat.Ino) // inodes are not equal
	require.Equal(t, fs.ModeSymlink, linkStat.Mode.Type())
	// From https://linux.die.net/man/2/lstat:
	// The size of a symbolic link is the length of the pathname it
	// contains, without a terminating null byte.
	if runtime.GOOS == "windows" { // size is zero, not the path length
		require.Zero(t, linkStat.Size)
	} else {
		require.Equal(t, int64(len(path)), linkStat.Size)
	}
}

func TestStat(t *testing.T) {
	tmpDir := t.TempDir()

	var stat Stat_t
	require.EqualErrno(t, syscall.ENOENT, Stat(pathutil.Join(tmpDir, "cat"), &stat))
	require.EqualErrno(t, syscall.ENOENT, Stat(pathutil.Join(tmpDir, "sub/cat"), &stat))

	t.Run("dir", func(t *testing.T) {
		err := Stat(tmpDir, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	file := pathutil.Join(tmpDir, "file")
	var statFile Stat_t

	t.Run("file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(file, nil, 0o400))
		require.NoError(t, Stat(file, &statFile))
		require.False(t, statFile.Mode.IsDir())
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		link := pathutil.Join(tmpDir, "file-link")
		require.NoError(t, os.Symlink(file, link))

		require.NoError(t, Stat(link, &stat))
		require.Equal(t, statFile, stat) // resolves to the file
	})

	subdir := pathutil.Join(tmpDir, "sub")
	var statSubdir Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		require.NoError(t, Stat(subdir, &statSubdir))
		require.True(t, statSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		link := pathutil.Join(tmpDir, "dir-link")
		require.NoError(t, os.Symlink(subdir, link))

		require.NoError(t, Stat(link, &stat))
		require.Equal(t, statSubdir, stat) // resolves to the dir
	})
}

func TestStatFile(t *testing.T) {
	tmpDir := t.TempDir()

	var stat Stat_t

	tmpDirF, err := OpenFile(tmpDir, syscall.O_RDONLY, 0)
	require.NoError(t, err)
	defer tmpDirF.Close()

	t.Run("dir", func(t *testing.T) {
		err = StatFile(tmpDirF, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
		requireDirectoryDevIno(t, stat)
	})

	// Windows allows you to stat a closed dir because it is accessed by path,
	// not by file descriptor.
	if runtime.GOOS != "windows" {
		t.Run("closed dir", func(t *testing.T) {
			require.NoError(t, tmpDirF.Close())
			require.EqualErrno(t, syscall.EBADF, StatFile(tmpDirF, &stat))
		})
	}

	file := pathutil.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o400))
	fileF, err := OpenFile(file, syscall.O_RDONLY, 0)
	require.NoError(t, err)
	defer fileF.Close()

	t.Run("file", func(t *testing.T) {
		err = StatFile(fileF, &stat)
		require.NoError(t, err)
		require.False(t, stat.Mode.IsDir())
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	t.Run("closed file", func(t *testing.T) {
		require.NoError(t, fileF.Close())
		require.EqualErrno(t, syscall.EBADF, StatFile(fileF, &stat))
		require.NotEqual(t, uint64(0), stat.Ino)
	})

	subdir := pathutil.Join(tmpDir, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o500))
	subdirF, err := OpenFile(subdir, syscall.O_RDONLY, 0)
	require.NoError(t, err)
	defer subdirF.Close()

	t.Run("subdir", func(t *testing.T) {
		err = StatFile(subdirF, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
		requireDirectoryDevIno(t, stat)
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed subdir", func(t *testing.T) {
			require.NoError(t, subdirF.Close())
			require.EqualErrno(t, syscall.EBADF, StatFile(subdirF, &stat))
		})
	}
}

func Test_StatFile_times(t *testing.T) {
	tmpDir := t.TempDir()

	file := pathutil.Join(tmpDir, "file")
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

			file, err := os.Open(file)
			require.NoError(t, err)
			defer file.Close()

			var stat Stat_t
			require.NoError(t, StatFile(file, &stat))
			require.Equal(t, stat.Atim, tc.atimeNsec)
			require.Equal(t, stat.Mtim, tc.mtimeNsec)
		})
	}
}

func TestStatFile_dev_inode(t *testing.T) {
	tmpDir := t.TempDir()
	d, err := os.Open(tmpDir)
	require.NoError(t, err)
	defer d.Close()

	path1 := pathutil.Join(tmpDir, "1")
	f1, err := os.Create(path1)
	require.NoError(t, err)
	defer f1.Close()

	path2 := pathutil.Join(tmpDir, "2")
	f2, err := os.Create(path2)
	require.NoError(t, err)
	defer f2.Close()

	pathLink2 := pathutil.Join(tmpDir, "link2")
	err = os.Symlink(path2, pathLink2)
	require.NoError(t, err)
	l2, err := os.Open(pathLink2)
	require.NoError(t, err)
	defer l2.Close()

	// First, stat the directory
	var stat1 Stat_t
	require.NoError(t, StatFile(d, &stat1))
	requireDirectoryDevIno(t, stat1)

	// Now, stat the files in it
	require.NoError(t, StatFile(f1, &stat1))

	var stat2 Stat_t
	require.NoError(t, StatFile(f2, &stat2))

	var stat3 Stat_t
	require.NoError(t, StatFile(l2, &stat3))

	// The files should be on the same device, but different inodes
	require.Equal(t, stat1.Dev, stat2.Dev)
	require.NotEqual(t, stat1.Ino, stat2.Ino)
	require.Equal(t, stat2, stat3) // stat on a link is for its target

	// Redoing stat should result in the same inodes
	var stat1Again Stat_t
	require.NoError(t, StatFile(f1, &stat1Again))
	require.Equal(t, stat1.Dev, stat1Again.Dev)

	// On Windows, we cannot rename while opening.
	// So we manually close here before renaming.
	require.NoError(t, f1.Close())
	require.NoError(t, f2.Close())
	require.NoError(t, l2.Close())

	// Renaming a file shouldn't change its inodes.
	require.NoError(t, Rename(path1, path2))
	f1, err = os.Open(path2)
	require.NoError(t, err)
	defer f1.Close()

	require.NoError(t, StatFile(f1, &stat1Again))
	require.Equal(t, stat1.Dev, stat1Again.Dev)
	require.Equal(t, stat1.Ino, stat1Again.Ino)
}

func requireDirectoryDevIno(t *testing.T, st Stat_t) {
	// windows before go 1.20 has trouble reading the inode information on directories.
	if runtime.GOOS != "windows" || IsGo120 {
		require.NotEqual(t, uint64(0), st.Dev)
		require.NotEqual(t, uint64(0), st.Ino)
	} else {
		require.Zero(t, st.Dev)
		require.Zero(t, st.Ino)
	}
}
