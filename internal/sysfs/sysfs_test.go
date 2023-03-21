package sysfs

import (
	"bytes"
	"embed"
	_ "embed"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"syscall"
	"testing"
	gofstest "testing/fstest"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func testOpen_O_RDWR(t *testing.T, tmpDir string, testFS FS) {
	file := "file"
	realPath := path.Join(tmpDir, file)
	err := os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	f, err := testFS.OpenFile(file, os.O_RDWR, 0)
	require.NoError(t, err)
	defer f.Close()

	w, ok := f.(io.Writer)
	require.True(t, ok)

	// If the write flag was honored, we should be able to write!
	fileContents := []byte{1, 2, 3, 4}
	n, err := w.Write(fileContents)
	require.NoError(t, err)
	require.Equal(t, len(fileContents), n)

	// Verify the contents actually wrote.
	b, err := os.ReadFile(realPath)
	require.NoError(t, err)
	require.Equal(t, fileContents, b)

	require.NoError(t, f.Close())

	// re-create as read-only, using 0444 to allow read-back on windows.
	require.NoError(t, os.Remove(realPath))
	f, err = testFS.OpenFile(file, os.O_RDONLY|os.O_CREATE, 0o444)
	require.NoError(t, err)
	defer f.Close()

	w, ok = f.(io.Writer)
	require.True(t, ok)

	if runtime.GOOS != "windows" {
		// If the read-only flag was honored, we should not be able to write!
		_, err = w.Write(fileContents)
		require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
	}

	// Verify stat on the file
	stat, err := f.Stat()
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o444), stat.Mode().Perm())

	// from os.TestDirFSPathsValid
	if runtime.GOOS != "windows" {
		t.Run("strange name", func(t *testing.T) {
			f, err := testFS.OpenFile(`e:xperi\ment.txt`, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
			require.NoError(t, err)
			defer f.Close()

			_, err = platform.StatFile(f)
			require.NoError(t, err)
		})
	}
}

func testOpen_Read(t *testing.T, testFS FS, expectIno bool) {
	t.Run("doesn't exist", func(t *testing.T) {
		_, err := testFS.OpenFile("nope", os.O_RDONLY, 0)

		// We currently follow os.Open not syscall.Open, so the error is wrapped.
		require.EqualErrno(t, syscall.ENOENT, err)
	})

	t.Run("readdir . opens root", func(t *testing.T) {
		f, err := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		dirents := requireReaddir(t, f, -1, expectIno)

		require.Equal(t, []*platform.Dirent{
			{Name: "animals.txt", Type: 0},
			{Name: "dir", Type: fs.ModeDir},
			{Name: "empty.txt", Type: 0},
			{Name: "emptydir", Type: fs.ModeDir},
			{Name: "sub", Type: fs.ModeDir},
		}, dirents)
	})

	t.Run("readdirnames . opens root", func(t *testing.T) {
		f, err := testFS.OpenFile(".", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		names := requireReaddirnames(t, f, -1)
		require.Equal(t, []string{"animals.txt", "dir", "empty.txt", "emptydir", "sub"}, names)
	})

	t.Run("readdir empty", func(t *testing.T) {
		f, err := testFS.OpenFile("emptydir", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		entries := requireReaddir(t, f, -1, expectIno)
		require.Zero(t, len(entries))
	})

	t.Run("readdirnames empty", func(t *testing.T) {
		f, err := testFS.OpenFile("emptydir", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		names := requireReaddirnames(t, f, -1)
		require.Zero(t, len(names))
	})

	t.Run("readdir partial", func(t *testing.T) {
		dirF, err := testFS.OpenFile("dir", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer dirF.Close()

		dirents1, err := platform.Readdir(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(dirents1))

		dirents2, err := platform.Readdir(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(dirents2))

		// read exactly the last entry
		dirents3, err := platform.Readdir(dirF, 1)
		require.NoError(t, err)
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
		_, err = platform.Readdir(dirF, 1)
		require.NoError(t, err)
	})

	// TODO: consolidate duplicated tests from platform once we have our own
	// file type
	t.Run("readdirnames partial", func(t *testing.T) {
		dirF, err := testFS.OpenFile("dir", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer dirF.Close()

		names1, err := platform.Readdirnames(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(names1))

		names2, err := platform.Readdirnames(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(names2))

		// read exactly the last entry
		names3, err := platform.Readdirnames(dirF, 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(names3))

		names := []string{names1[0], names2[0], names3[0]}
		sort.Strings(names)

		require.Equal(t, []string{"-", "a-", "ab-"}, names)

		// no error reading an exhausted directory
		_, err = platform.Readdirnames(dirF, 1)
		require.NoError(t, err)
	})

	t.Run("file exists", func(t *testing.T) {
		f, err := testFS.OpenFile("animals.txt", os.O_RDONLY, 0)
		require.NoError(t, err)
		defer f.Close()

		fileContents := []byte(`bear
cat
shark
dinosaur
human
`)
		// Ensure it implements io.ReaderAt
		r, ok := f.(io.ReaderAt)
		require.True(t, ok)
		lenToRead := len(fileContents) - 1
		buf := make([]byte, lenToRead)
		n, err := r.ReadAt(buf, 1)
		require.NoError(t, err)
		require.Equal(t, lenToRead, n)
		require.Equal(t, fileContents[1:], buf)

		// Ensure it implements io.Seeker
		s, ok := f.(io.Seeker)
		require.True(t, ok)
		offset, err := s.Seek(1, io.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(1), offset)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		require.Equal(t, fileContents[1:], b)
	})

	// Make sure O_RDONLY isn't treated bitwise as it is usually zero.
	t.Run("or'd flag", func(t *testing.T) {
		// Example of a flag that can be or'd into O_RDONLY even if not
		// currently supported in WASI or GOOS=js
		const O_NOATIME = 0x40000

		f, err := testFS.OpenFile("animals.txt", os.O_RDONLY|O_NOATIME, 0)
		require.NoError(t, err)
		defer f.Close()
	})

	t.Run("writing to a read-only file is EBADF", func(t *testing.T) {
		f, err := testFS.OpenFile("animals.txt", os.O_RDONLY, 0)
		defer require.NoError(t, f.Close())
		require.NoError(t, err)

		if w, ok := f.(io.Writer); ok {
			_, err = w.Write([]byte{1, 2, 3, 4})
			require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
		} else {
			t.Skip("not an io.Writer")
		}
	})

	t.Run("writing to a directory is EBADF", func(t *testing.T) {
		f, err := testFS.OpenFile("sub", os.O_RDONLY, 0)
		defer require.NoError(t, f.Close())
		require.NoError(t, err)

		if w, ok := f.(io.Writer); ok {
			_, err = w.Write([]byte{1, 2, 3, 4})
			require.EqualErrno(t, syscall.EBADF, platform.UnwrapOSError(err))
		} else {
			t.Skip("not an io.Writer")
		}
	})
}

func testLstat(t *testing.T, testFS FS) {
	_, err := testFS.Lstat("cat")
	require.EqualErrno(t, syscall.ENOENT, err)
	_, err = testFS.Lstat("sub/cat")
	require.EqualErrno(t, syscall.ENOENT, err)

	var st platform.Stat_t

	t.Run("dir", func(t *testing.T) {
		st, err = testFS.Lstat(".")
		require.NoError(t, err)
		require.True(t, st.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	var stFile platform.Stat_t

	t.Run("file", func(t *testing.T) {
		stFile, err = testFS.Lstat("animals.txt")
		require.NoError(t, err)
		require.Zero(t, stFile.Mode.Type())
		require.Equal(t, int64(30), stFile.Size)
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to file", func(t *testing.T) {
		requireLinkStat(t, testFS, "animals.txt", stFile)
	})

	var stSubdir platform.Stat_t
	t.Run("subdir", func(t *testing.T) {
		stSubdir, err = testFS.Lstat("sub")
		require.NoError(t, err)
		require.True(t, stSubdir.Mode.IsDir())
		require.NotEqual(t, uint64(0), st.Ino)
	})

	t.Run("link to dir", func(t *testing.T) {
		requireLinkStat(t, testFS, "sub", stSubdir)
	})

	t.Run("link to dir link", func(t *testing.T) {
		pathLink := "sub-link"
		stLink, err := testFS.Lstat(pathLink)
		require.NoError(t, err)

		requireLinkStat(t, testFS, pathLink, stLink)
	})
}

func requireLinkStat(t *testing.T, testFS FS, path string, stat platform.Stat_t) {
	link := path + "-link"
	stLink, err := testFS.Lstat(link)
	require.NoError(t, err)
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
	_, err := testFS.Stat("cat")
	require.EqualErrno(t, syscall.ENOENT, err)
	_, err = testFS.Stat("sub/cat")
	require.EqualErrno(t, syscall.ENOENT, err)

	st, err := testFS.Stat("sub/test.txt")
	require.NoError(t, err)
	require.False(t, st.Mode.IsDir())
	require.NotEqual(t, uint64(0), st.Dev)
	require.NotEqual(t, uint64(0), st.Ino)

	st, err = testFS.Stat("sub")
	require.NoError(t, err)
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
	entries, err := platform.Readdir(f, n)
	require.NoError(t, err)
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
	names, err := platform.Readdirnames(f, n)
	require.NoError(t, err)
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
		err := writeFS.Symlink(tl.old, tl.dst) // not os.Symlink for windows compat
		require.NoError(t, err, "%v", tl)

		dst, err := readFS.Readlink(tl.dst)
		require.NoError(t, err)
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

var (
	//go:embed testdata
	testdata     embed.FS
	readerAtFile = "wazero.txt"
	emptyFile    = "empty.txt"
)

func TestReaderAtOffset(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	d, err := embedFS.Open(readerAtFile)
	require.NoError(t, err)
	defer d.Close()

	bytes, err := io.ReadAll(d)
	require.NoError(t, err)

	mapFS := gofstest.MapFS{readerAtFile: &gofstest.MapFile{Data: bytes}}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(tmpDir, readerAtFile), bytes, 0o600))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.Open(readerAtFile)
			require.NoError(t, err)
			defer f.Close()

			var r io.Reader = f
			ra := ReaderAtOffset(f, 0)

			requireRead3 := func(r io.Reader, buf []byte) {
				n, err := r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 3, n)
			}

			// The file should work as a reader (base case)
			requireRead3(r, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// The readerAt impl should be able to start from zero also
			requireRead3(ra, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// If the offset didn't change, we expect the next three chars.
			requireRead3(r, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// If state was held between reader-at, we expect the same
			requireRead3(ra, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// We should also be able to make another reader-at
			ra = ReaderAtOffset(f, 3)
			requireRead3(ra, buf)
			require.Equal(t, "ero", string(buf))
		})
	}
}

func TestReaderAtOffset_empty(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	d, err := embedFS.Open(readerAtFile)
	require.NoError(t, err)
	defer d.Close()

	mapFS := gofstest.MapFS{emptyFile: &gofstest.MapFile{}}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(path.Join(tmpDir, emptyFile), []byte{}, 0o600))
	dirFS := os.DirFS(tmpDir)

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.Open(emptyFile)
			require.NoError(t, err)
			defer f.Close()

			var r io.Reader = f
			ra := ReaderAtOffset(f, 0)

			requireRead3 := func(r io.Reader, buf []byte) {
				n, err := r.Read(buf)
				require.Equal(t, err, io.EOF)
				require.Equal(t, 0, n) // file is empty
			}

			// The file should work as a reader (base case)
			requireRead3(r, buf)

			// The readerAt impl should be able to start from zero also
			requireRead3(ra, buf)
		})
	}
}

func TestReaderAtOffset_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(emptyFile)
	require.NoError(t, err)
	defer f.Close()

	// mask both io.ReaderAt and io.Seeker
	ra := ReaderAtOffset(struct{ fs.File }{f}, 0)

	buf := make([]byte, 3)
	_, err = ra.Read(buf)
	require.Equal(t, syscall.ENOSYS, err)
}

func TestWriterAtOffset(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS := NewDirFS(tmpDir)

	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	tests := []struct {
		name string
		fs   FS
	}{
		{name: "sysfs.dirFS", fs: dirFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.OpenFile(readerAtFile, os.O_RDWR|os.O_CREATE, 0o600)
			require.NoError(t, err)
			defer f.Close()

			w := f.(io.Writer)
			wa := WriterAtOffset(f, 6)

			text := "wazero"
			buf := make([]byte, 3)
			copy(buf, text[:3])

			requireWrite3 := func(r io.Writer, buf []byte) {
				n, err := r.Write(buf)
				require.NoError(t, err)
				require.Equal(t, 3, n)
			}

			// The file should work as a writer (base case)
			requireWrite3(w, buf)

			// The writerAt impl should be able to start from zero also
			requireWrite3(wa, buf)

			copy(buf, text[3:])

			// If the offset didn't change, the next chars will write after the
			// first
			requireWrite3(w, buf)

			// If state was held between writer-at, we expect the same
			requireWrite3(wa, buf)

			// We should also be able to make another writer-at
			wa = WriterAtOffset(f, 12)
			requireWrite3(wa, buf)

			r := ReaderAtOffset(f, 0)
			b, err := io.ReadAll(r)
			require.NoError(t, err)

			// We expect to have written the text two and a half times:
			//  1. io.Write: offset 0
			//  2. io.WriterAt: offset 6
			//  3. second io.WriterAt: offset 12, writing "ero"
			require.Equal(t, text+text+text[3:], string(b))
		})
	}
}

func TestWriterAtOffset_empty(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS := NewDirFS(tmpDir)

	// fs.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	tests := []struct {
		name string
		fs   FS
	}{
		{name: "sysfs.dirFS", fs: dirFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.OpenFile(emptyFile, os.O_RDWR|os.O_CREATE, 0o600)
			require.NoError(t, err)
			defer f.Close()

			r := f.(io.Writer)
			ra := WriterAtOffset(f, 0)

			var emptyBuf []byte

			requireWrite := func(r io.Writer) {
				n, err := r.Write(emptyBuf)
				require.NoError(t, err)
				require.Equal(t, 0, n) // file is empty
			}

			// The file should work as a writer (base case)
			requireWrite(r)

			// The writerAt impl should be able to start from zero also
			requireWrite(ra)
		})
	}
}

func TestWriterAtOffset_Unsupported(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS := NewDirFS(tmpDir)

	f, err := dirFS.OpenFile(readerAtFile, os.O_RDWR|os.O_CREATE, 0o600)
	require.NoError(t, err)
	defer f.Close()

	// mask both io.WriterAt and io.Seeker
	ra := WriterAtOffset(struct{ fs.File }{f}, 0)

	buf := make([]byte, 3)
	_, err = ra.Write(buf)
	require.Equal(t, syscall.ENOSYS, err)
}

// Test_FileSync doesn't guarantee sync works because the operating system may
// sync anyway. There is no test in Go for os.File Sync, but closest is similar
// to below. Effectively, this only tests that things don't error.
func Test_FileSync(t *testing.T) {
	testSync(t, func(f fs.File) error {
		return f.(interface{ Sync() error }).Sync()
	})
}

// Test_FileDatasync has same issues as Test_Sync.
func Test_FileDatasync(t *testing.T) {
	testSync(t, FileDatasync)
}

func testSync(t *testing.T, sync func(fs.File) error) {
	f, err := os.CreateTemp("", t.Name())
	require.NoError(t, err)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, err = f.Write([]byte(expected))
	require.NoError(t, err)

	// Sync the data.
	require.NoError(t, sync(f))

	// Rewind while the file is still open.
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Read data from the file
	var buf bytes.Buffer
	_, err = io.Copy(&buf, f)
	require.NoError(t, err)

	// It may be the case that sync worked.
	require.Equal(t, expected, buf.String())
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
