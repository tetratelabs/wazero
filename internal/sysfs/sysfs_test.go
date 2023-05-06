package sysfs

import (
	_ "embed"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func testOpen_O_RDWR(t *testing.T, tmpDir string, testFS FS) {
	file := "file"
	realPath := path.Join(tmpDir, file)
	err := os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	f, errno := testFS.OpenFile(file, os.O_RDWR, 0)
	require.EqualErrno(t, 0, errno)
	defer f.Close()

	// If the write flag was honored, we should be able to write!
	fileContents := []byte{1, 2, 3, 4}
	n, errno := f.Write(fileContents)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(fileContents), n)

	// Verify the contents actually wrote.
	b, err := os.ReadFile(realPath)
	require.NoError(t, err)
	require.Equal(t, fileContents, b)

	require.Zero(t, f.Close())

	// re-create as read-only, using 0444 to allow read-back on windows.
	require.NoError(t, os.Remove(realPath))
	f, errno = testFS.OpenFile(file, os.O_RDONLY|os.O_CREATE, 0o444)
	require.EqualErrno(t, 0, errno)
	defer f.Close()

	if runtime.GOOS != "windows" {
		// If the read-only flag was honored, we should not be able to write!
		_, err = f.Write(fileContents)
		require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
	}

	// Verify stat on the file
	stat, errno := f.Stat()
	require.EqualErrno(t, 0, errno)
	require.Equal(t, fs.FileMode(0o444), stat.Mode.Perm())

	// from os.TestDirFSPathsValid
	if runtime.GOOS != "windows" {
		t.Run("strange name", func(t *testing.T) {
			f, errno = testFS.OpenFile(`e:xperi\ment.txt`, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
			require.EqualErrno(t, 0, errno)
			defer f.Close()

			_, errno = f.Stat()
			require.EqualErrno(t, 0, errno)
		})
	}
}

func testOpen_Read(t *testing.T, testFS FS, expectIno bool) {
	t.Run("doesn't exist", func(t *testing.T) {
		_, errno := testFS.OpenFile("nope", os.O_RDONLY, 0)

		// We currently follow os.Open not syscall.Open, so the error is wrapped.
		require.EqualErrno(t, syscall.ENOENT, errno)
	})

	t.Run("readdir . opens root", func(t *testing.T) {
		f, errno := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		dirents := requireReaddir(t, f.File(), -1, expectIno)

		require.Equal(t, []*platform.Dirent{
			{Name: "animals.txt", Type: 0},
			{Name: "dir", Type: fs.ModeDir},
			{Name: "empty.txt", Type: 0},
			{Name: "emptydir", Type: fs.ModeDir},
			{Name: "sub", Type: fs.ModeDir},
		}, dirents)
	})

	t.Run("readdirnames . opens root", func(t *testing.T) {
		f, errno := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		names := requireReaddirnames(t, f.File(), -1)
		require.Equal(t, []string{"animals.txt", "dir", "empty.txt", "emptydir", "sub"}, names)
	})

	t.Run("readdir empty", func(t *testing.T) {
		f, errno := testFS.OpenFile("emptydir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		entries := requireReaddir(t, f.File(), -1, expectIno)
		require.Zero(t, len(entries))
	})

	t.Run("readdirnames empty", func(t *testing.T) {
		f, errno := testFS.OpenFile("emptydir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		names := requireReaddirnames(t, f.File(), -1)
		require.Zero(t, len(names))
	})

	t.Run("readdir partial", func(t *testing.T) {
		dirF, errno := testFS.OpenFile("dir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer dirF.Close()

		dirents1, errno := platform.Readdir(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents1))

		dirents2, errno := platform.Readdir(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents2))

		// read exactly the last entry
		dirents3, errno := platform.Readdir(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents3))

		dirents := []*platform.Dirent{dirents1[0], dirents2[0], dirents3[0]}
		sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

		requireIno(t, dirents, expectIno)

		require.Equal(t, []*platform.Dirent{
			{Name: "-", Type: 0},
			{Name: "a-", Type: fs.ModeDir},
			{Name: "ab-", Type: 0},
		}, dirents)

		// no error reading an exhausted directory
		_, errno = platform.Readdir(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
	})

	// TODO: consolidate duplicated tests from platform once we have our own
	// file type
	t.Run("readdirnames partial", func(t *testing.T) {
		dirF, errno := testFS.OpenFile("dir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer dirF.Close()

		names1, errno := platform.Readdirnames(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(names1))

		names2, errno := platform.Readdirnames(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(names2))

		// read exactly the last entry
		names3, errno := platform.Readdirnames(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(names3))

		names := []string{names1[0], names2[0], names3[0]}
		sort.Strings(names)

		require.Equal(t, []string{"-", "a-", "ab-"}, names)

		// no error reading an exhausted directory
		_, errno = platform.Readdirnames(dirF.File(), 1)
		require.EqualErrno(t, 0, errno)
	})

	t.Run("file exists", func(t *testing.T) {
		f, errno := testFS.OpenFile("animals.txt", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		fileContents := []byte(`bear
cat
shark
dinosaur
human
`)
		// Ensure it implements io.ReaderAt
		r, ok := f.File().(io.ReaderAt)
		require.True(t, ok)
		lenToRead := len(fileContents) - 1
		buf := make([]byte, lenToRead)
		n, err := r.ReadAt(buf, 1)
		require.NoError(t, err)
		require.Equal(t, lenToRead, n)
		require.Equal(t, fileContents[1:], buf)

		// Ensure it implements io.Seeker
		s, ok := f.File().(io.Seeker)
		require.True(t, ok)
		offset, err := s.Seek(1, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(1), offset)
		b, err := io.ReadAll(f.File())
		require.NoError(t, err)
		require.Equal(t, fileContents[1:], b)
	})

	// Make sure O_RDONLY isn't treated bitwise as it is usually zero.
	t.Run("or'd flag", func(t *testing.T) {
		// Example of a flag that can be or'd into O_RDONLY even if not
		// currently supported in WASI or GOOS=js
		const O_NOATIME = 0x40000

		f, errno := testFS.OpenFile("animals.txt", os.O_RDONLY|O_NOATIME, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()
	})

	t.Run("writing to a read-only file is EBADF", func(t *testing.T) {
		f, errno := testFS.OpenFile("animals.txt", os.O_RDONLY, 0)
		defer require.Zero(t, f.Close())
		require.EqualErrno(t, 0, errno)

		_, err := f.Write([]byte{1, 2, 3, 4})
		require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
	})

	t.Run("writing to a directory is EBADF", func(t *testing.T) {
		f, errno := testFS.OpenFile("sub", os.O_RDONLY, 0)
		defer require.Zero(t, f.Close())
		require.EqualErrno(t, 0, errno)

		_, err := f.Write([]byte{1, 2, 3, 4})
		require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
	})
}

func testLstat(t *testing.T, testFS FS) {
	_, errno := testFS.Lstat("cat")
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = testFS.Lstat("sub/cat")
	require.EqualErrno(t, syscall.ENOENT, errno)

	var st platform.Stat_t

	t.Run("dir", func(t *testing.T) {
		st, errno = testFS.Lstat(".")
		require.EqualErrno(t, 0, errno)
		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	var stFile platform.Stat_t

	t.Run("file", func(t *testing.T) {
		stFile, errno = testFS.Lstat("animals.txt")
		require.EqualErrno(t, 0, errno)

		require.Zero(t, stFile.Mode.Type())
		require.Equal(t, int64(30), stFile.Size)
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		requireLinkStat(t, testFS, "animals.txt", stFile)
	})

	var stSubdir platform.Stat_t
	t.Run("subdir", func(t *testing.T) {
		stSubdir, errno = testFS.Lstat("sub")
		require.EqualErrno(t, 0, errno)

		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		requireLinkStat(t, testFS, "sub", stSubdir)
	})

	t.Run("link to dir link", func(t *testing.T) {
		pathLink := "sub-link"
		stLink, errno := testFS.Lstat(pathLink)
		require.EqualErrno(t, 0, errno)

		requireLinkStat(t, testFS, pathLink, stLink)
	})
}

func requireLinkStat(t *testing.T, testFS FS, path string, stat platform.Stat_t) {
	link := path + "-link"
	stLink, errno := testFS.Lstat(link)
	require.EqualErrno(t, 0, errno)

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

func testStat(t *testing.T, testFS FS) {
	_, errno := testFS.Stat("cat")
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = testFS.Stat("sub/cat")
	require.EqualErrno(t, syscall.ENOENT, errno)

	st, errno := testFS.Stat("sub/test.txt")
	require.EqualErrno(t, 0, errno)

	require.False(t, st.Mode.IsDir())
	require.NotEqual(t, uint64(0), st.Dev)
	require.NotEqual(t, uint64(0), st.Ino)

	st, errno = testFS.Stat("sub")
	require.EqualErrno(t, 0, errno)

	require.True(t, st.Mode.IsDir())
	// windows before go 1.20 has trouble reading the inode information on directories.
	if runtime.GOOS != "windows" || platform.IsGo120 {
		require.NotEqual(t, uint64(0), st.Dev)
		require.NotEqual(t, uint64(0), st.Ino)
	}
}

// requireReaddir ensures the input file is a directory, and returns its
// entries.
func requireReaddir(t *testing.T, f fs.File, n int, expectIno bool) []*platform.Dirent {
	entries, errno := platform.Readdir(f, n)
	require.EqualErrno(t, 0, errno)

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	if _, ok := f.(*openRootDir); ok {
		// TODO: get inodes to work on the root directory of a composite FS
		requireIno(t, entries, false)
	} else {
		requireIno(t, entries, expectIno)
	}
	return entries
}

// requireReaddirnames ensures the input file is a directory, and returns its
// entries.
func requireReaddirnames(t *testing.T, f fs.File, n int) []string {
	names, errno := platform.Readdirnames(f, n)
	require.EqualErrno(t, 0, errno)
	sort.Strings(names)
	return names
}

func testReadlink(t *testing.T, readFS, writeFS FS) {
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

	for _, tl := range testLinks {
		errno := writeFS.Symlink(tl.old, tl.dst) // not os.Symlink for windows compat
		require.Zero(t, errno, "%v", tl)

		dst, errno := readFS.Readlink(tl.dst)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, tl.old, dst)
	}

	t.Run("errors", func(t *testing.T) {
		_, err := readFS.Readlink("sub/test.txt")
		require.Error(t, err)
		_, err = readFS.Readlink("")
		require.Error(t, err)
		_, err = readFS.Readlink("animals.txt")
		require.Error(t, err)
	})
}

func requireIno(t *testing.T, dirents []*platform.Dirent, expectIno bool) {
	for _, e := range dirents {
		if expectIno {
			require.NotEqual(t, uint64(0), e.Ino, "%+v", e)
			e.Ino = 0
		} else {
			require.Zero(t, e.Ino, "%+v", e)
		}
	}
}

// joinPath avoids us having to rename fields just to avoid conflict with the
// path package.
func joinPath(dirName, baseName string) string {
	return path.Join(dirName, baseName)
}
