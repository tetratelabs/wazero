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

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func testOpen_O_RDWR(t *testing.T, tmpDir string, testFS fsapi.FS) {
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

	require.EqualErrno(t, 0, f.Close())

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

	t.Run("O_TRUNC", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFS := NewDirFS(tmpDir)

		name := "truncate"
		realPath := path.Join(tmpDir, name)
		require.NoError(t, os.WriteFile(realPath, []byte("123456"), 0o0666))

		f, errno = testFS.OpenFile(name, os.O_RDWR|os.O_TRUNC, 0o444)
		require.EqualErrno(t, 0, errno)
		require.EqualErrno(t, 0, f.Close())

		actual, err := os.ReadFile(realPath)
		require.NoError(t, err)
		require.Equal(t, 0, len(actual))
	})
}

func testOpen_Read(t *testing.T, testFS fsapi.FS, requireFileIno, expectDirIno bool) {
	t.Helper()

	t.Run("doesn't exist", func(t *testing.T) {
		_, errno := testFS.OpenFile("nope", os.O_RDONLY, 0)

		// We currently follow os.Open not syscall.Open, so the error is wrapped.
		require.EqualErrno(t, syscall.ENOENT, errno)
	})

	t.Run("readdir . opens root", func(t *testing.T) {
		f, errno := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		dirents := requireReaddir(t, f, -1, expectDirIno)

		// Scrub inodes so we can compare expectations without them.
		for i := range dirents {
			dirents[i].Ino = 0
		}

		require.Equal(t, []fsapi.Dirent{
			{Name: "animals.txt", Type: 0},
			{Name: "dir", Type: fs.ModeDir},
			{Name: "empty.txt", Type: 0},
			{Name: "emptydir", Type: fs.ModeDir},
			{Name: "sub", Type: fs.ModeDir},
		}, dirents)
	})

	t.Run("readdir empty", func(t *testing.T) {
		f, errno := testFS.OpenFile("emptydir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		entries := requireReaddir(t, f, -1, expectDirIno)
		require.Zero(t, len(entries))
	})

	t.Run("readdir partial", func(t *testing.T) {
		dirF, errno := testFS.OpenFile("dir", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer dirF.Close()

		dirents1, errno := dirF.Readdir(1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents1))

		dirents2, errno := dirF.Readdir(1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents2))

		// read exactly the last entry
		dirents3, errno := dirF.Readdir(1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, 1, len(dirents3))

		dirents := []fsapi.Dirent{dirents1[0], dirents2[0], dirents3[0]}
		sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name < dirents[j].Name })

		requireIno(t, dirents, expectDirIno)

		// Scrub inodes so we can compare expectations without them.
		for i := range dirents {
			dirents[i].Ino = 0
		}

		require.Equal(t, []fsapi.Dirent{
			{Name: "-", Type: 0},
			{Name: "a-", Type: fs.ModeDir},
			{Name: "ab-", Type: 0},
		}, dirents)

		// no error reading an exhausted directory
		_, errno = dirF.Readdir(1)
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
		// Ensure it implements Pread
		lenToRead := len(fileContents) - 1
		buf := make([]byte, lenToRead)
		n, errno := f.Pread(buf, 1)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, lenToRead, n)
		require.Equal(t, fileContents[1:], buf)

		// Ensure it implements Seek
		offset, errno := f.Seek(1, io.SeekStart)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, int64(1), offset)

		// Read should pick up from position 1
		n, errno = f.Read(buf)
		require.EqualErrno(t, 0, errno)
		require.Equal(t, lenToRead, n)
		require.Equal(t, fileContents[1:], buf)
	})

	t.Run("file stat includes inode", func(t *testing.T) {
		f, errno := testFS.OpenFile("empty.txt", os.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		st, errno := f.Stat()
		require.EqualErrno(t, 0, errno)

		// Results are inconsistent, so don't validate the opposite.
		if requireFileIno {
			require.NotEqual(t, uint64(0), st.Ino, "%+v", st)
		}
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
		require.EqualErrno(t, 0, errno)
		defer f.Close()

		_, errno = f.Write([]byte{1, 2, 3, 4})
		require.EqualErrno(t, syscall.EBADF, errno)
	})

	t.Run("opening a directory with O_RDWR is EISDIR", func(t *testing.T) {
		_, errno := testFS.OpenFile("sub", fsapi.O_DIRECTORY|os.O_RDWR, 0)
		require.EqualErrno(t, syscall.EISDIR, errno)
	})
}

func testLstat(t *testing.T, testFS fsapi.FS) {
	_, errno := testFS.Lstat("cat")
	require.EqualErrno(t, syscall.ENOENT, errno)
	_, errno = testFS.Lstat("sub/cat")
	require.EqualErrno(t, syscall.ENOENT, errno)

	var st sys.Stat_t

	t.Run("dir", func(t *testing.T) {
		st, errno = testFS.Lstat(".")
		require.EqualErrno(t, 0, errno)
		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	var stFile sys.Stat_t

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

	var stSubdir sys.Stat_t
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

func requireLinkStat(t *testing.T, testFS fsapi.FS, path string, stat sys.Stat_t) {
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

func testStat(t *testing.T, testFS fsapi.FS) {
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
func requireReaddir(t *testing.T, f fsapi.File, n int, expectDirIno bool) []fsapi.Dirent {
	entries, errno := f.Readdir(n)
	require.EqualErrno(t, 0, errno)

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	requireIno(t, entries, expectDirIno)
	return entries
}

func testReadlink(t *testing.T, readFS, writeFS fsapi.FS) {
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

func requireIno(t *testing.T, dirents []fsapi.Dirent, expectDirIno bool) {
	for i := range dirents {
		d := dirents[i]
		if expectDirIno {
			require.NotEqual(t, uint64(0), d.Ino, "%+v", d)
			d.Ino = 0
		} else {
			require.Zero(t, d.Ino, "%+v", d)
		}
	}
}

// joinPath avoids us having to rename fields just to avoid conflict with the
// path package.
func joinPath(dirName, baseName string) string {
	return path.Join(dirName, baseName)
}
