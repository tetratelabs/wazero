package sysfs

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func TestStat(t *testing.T) {
	tmpDir := t.TempDir()

	_, errno := stat(path.Join(tmpDir, "cat"))
	require.EqualErrno(t, experimentalsys.ENOENT, errno)
	_, errno = stat(path.Join(tmpDir, "sub/cat"))
	require.EqualErrno(t, experimentalsys.ENOENT, errno)

	var st sys.Stat_t

	t.Run("empty dir", func(t *testing.T) {
		st, errno = stat(tmpDir)
		require.EqualErrno(t, 0, errno)

		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)

		// We expect one link: the directory itself
		expectedNlink := uint64(1)
		if dirNlinkIncludesDot {
			expectedNlink++
		}
		require.Equal(t, expectedNlink, st.Nlink, runtime.GOOS)
	})

	subdir := path.Join(tmpDir, "sub")
	var stSubdir sys.Stat_t
	t.Run("subdir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(subdir, 0o500))

		stSubdir, errno = stat(subdir)
		require.EqualErrno(t, 0, errno)

		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("not empty dir", func(t *testing.T) {
		st, errno = stat(tmpDir)
		require.EqualErrno(t, 0, errno)

		// We expect two links: the directory itself and the subdir
		expectedNlink := uint64(2)
		if dirNlinkIncludesDot {
			expectedNlink++
		} else if runtime.GOOS == "windows" {
			expectedNlink = 1 // directory count is not returned.
		}
		require.Equal(t, expectedNlink, st.Nlink, runtime.GOOS)
	})

	// TODO: Investigate why Nlink increases on BSD when a file is added, but
	// not Linux.

	file := path.Join(tmpDir, "file")
	var stFile sys.Stat_t

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

	tmpDirF := requireOpenFile(t, tmpDir, experimentalsys.O_RDONLY, 0)
	defer tmpDirF.Close()

	t.Run("dir", func(t *testing.T) {
		st, errno := tmpDirF.Stat()
		require.EqualErrno(t, 0, errno)
		requireDir(t, tmpDirF, st)
		requireDevIno(t, tmpDirF, st)
	})

	// Windows allows you to stat a closed dir because it is accessed by path,
	// not by file descriptor.
	if runtime.GOOS != "windows" {
		t.Run("closed dir", func(t *testing.T) {
			require.EqualErrno(t, 0, tmpDirF.Close())
			_, errno := tmpDirF.Stat()
			require.EqualErrno(t, experimentalsys.EBADF, errno)
		})
	}

	file := path.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o400))
	fileF := requireOpenFile(t, file, experimentalsys.O_RDONLY, 0)
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
		require.EqualErrno(t, experimentalsys.EBADF, errno)
	})

	subdir := path.Join(tmpDir, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o500))
	subdirF := requireOpenFile(t, subdir, experimentalsys.O_RDONLY, 0)
	defer subdirF.Close()

	t.Run("subdir", func(t *testing.T) {
		st, errno := subdirF.Stat()
		require.EqualErrno(t, 0, errno)
		requireDir(t, subdirF, st)
		requireDevIno(t, subdirF, st)
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed subdir", func(t *testing.T) {
			require.EqualErrno(t, 0, subdirF.Close())
			_, errno := subdirF.Stat()
			require.EqualErrno(t, experimentalsys.EBADF, errno)
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

			f := requireOpenFile(t, file, experimentalsys.O_RDONLY, 0)
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
	d := requireOpenFile(t, tmpDir, experimentalsys.O_RDONLY, 0)
	defer d.Close()

	path1 := path.Join(tmpDir, "1")
	f1 := requireOpenFile(t, path1, experimentalsys.O_CREAT, 0o666)
	defer f1.Close()

	path2 := path.Join(tmpDir, "2")
	f2 := requireOpenFile(t, path2, experimentalsys.O_CREAT, 0o666)
	defer f2.Close()

	pathLink2 := path.Join(tmpDir, "link2")
	err := os.Symlink(path2, pathLink2)
	require.NoError(t, err)
	l2 := requireOpenFile(t, pathLink2, experimentalsys.O_RDONLY, 0)
	defer l2.Close()

	// First, stat the directory
	st1, errno := d.Stat()
	require.EqualErrno(t, 0, errno)
	requireDir(t, d, st1)
	requireDevIno(t, d, st1)

	// Now, stat the files in it
	st1, errno = f1.Stat()
	require.EqualErrno(t, 0, errno)
	requireNotDir(t, f1, st1)
	requireDevIno(t, f1, st1)

	st2, errno := f2.Stat()
	require.EqualErrno(t, 0, errno)
	requireNotDir(t, f2, st2)
	requireDevIno(t, f2, st2)

	st3, errno := l2.Stat()
	require.EqualErrno(t, 0, errno)
	requireNotDir(t, l2, st3)
	requireDevIno(t, l2, st3)

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
	require.EqualErrno(t, 0, rename(path1, path2))
	f1 = requireOpenFile(t, path2, experimentalsys.O_RDONLY, 0)
	defer f1.Close()

	st1Again, errno = f1.Stat()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, st1.Dev, st1Again.Dev)
	require.Equal(t, st1.Ino, st1Again.Ino)
}

func requireNotDir(t *testing.T, d experimentalsys.File, st sys.Stat_t) {
	// Verify cached state is correct
	isDir, errno := d.IsDir()
	require.EqualErrno(t, 0, errno)
	require.False(t, isDir)
	require.False(t, st.Mode.IsDir())
}

func requireDir(t *testing.T, d experimentalsys.File, st sys.Stat_t) {
	// Verify cached state is correct
	isDir, errno := d.IsDir()
	require.EqualErrno(t, 0, errno)
	require.True(t, isDir)
	require.True(t, st.Mode.IsDir())
}

func requireDevIno(t *testing.T, f experimentalsys.File, st sys.Stat_t) {
	// Results are inconsistent, so don't validate the opposite.
	require.NotEqual(t, uint64(0), st.Dev)
	require.NotEqual(t, uint64(0), st.Ino)

	// Verify the special-cased properties supporting wasip2 "is_same_object"
	// See https://github.com/WebAssembly/wasi-filesystem/pull/81
	dev, errno := f.Dev()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, st.Dev, dev)
	ino, errno := f.Ino()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, st.Ino, ino)
}
