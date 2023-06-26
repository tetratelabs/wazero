package sysfs

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"
	"syscall"
	"testing"
	gofstest "testing/fstest"
	"time"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed file_test.go
var embedFS embed.FS

var (
	//go:embed testdata
	testdata   embed.FS
	wazeroFile = "wazero.txt"
	emptyFile  = "empty.txt"
)

func TestStdioFileSetNonblock(t *testing.T) {
	// Test using os.Pipe as it is known to support non-blocking reads.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	rF, err := NewStdioFile(true, r)
	require.NoError(t, err)

	errno := rF.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, rF.IsNonblock())

	errno = rF.SetNonblock(false)
	require.EqualErrno(t, 0, errno)
	require.False(t, rF.IsNonblock())
}

func TestRegularFileSetNonblock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Nonblock on regular files is not supported on Windows")
	}

	// Test using os.Pipe as it is known to support non-blocking reads.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	rF := newOsFile("", syscall.O_RDONLY, 0, r)

	errno := rF.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, rF.IsNonblock())

	// Read from the file without ever writing to it should not block.
	buf := make([]byte, 8)
	_, e := rF.Read(buf)
	require.EqualErrno(t, syscall.EAGAIN, e)

	errno = rF.SetNonblock(false)
	require.EqualErrno(t, 0, errno)
	require.False(t, rF.IsNonblock())
}

func TestReadFdNonblock(t *testing.T) {
	// Test using os.Pipe as it is known to support non-blocking reads.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	fd := r.Fd()
	err = setNonblock(fd, true)
	require.NoError(t, err)

	// Read from the file without ever writing to it should not block.
	buf := make([]byte, 8)
	_, e := readFd(fd, buf)
	if runtime.GOOS == "windows" {
		require.EqualErrno(t, syscall.ENOSYS, e)
	} else {
		require.EqualErrno(t, syscall.EAGAIN, e)
	}
}

func TestFileSetAppend(t *testing.T) {
	tmpDir := t.TempDir()

	fPath := path.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(fPath, []byte("0123456789"), 0o600))

	// Open without APPEND.
	f, errno := OpenOSFile(fPath, os.O_RDWR, 0o600)
	require.EqualErrno(t, 0, errno)
	require.False(t, f.IsAppend())

	// Set the APPEND flag.
	require.EqualErrno(t, 0, f.SetAppend(true))
	require.True(t, f.IsAppend())

	requireFileContent := func(exp string) {
		buf, err := os.ReadFile(fPath)
		require.NoError(t, err)
		require.Equal(t, exp, string(buf))
	}

	// with O_APPEND flag, the data is appended to buffer.
	_, errno = f.Write([]byte("wazero"))
	require.EqualErrno(t, 0, errno)
	requireFileContent("0123456789wazero")

	// Remove the APPEND flag.
	require.EqualErrno(t, 0, f.SetAppend(false))
	require.False(t, f.IsAppend())

	// without O_APPEND flag, the data writes at offset zero
	_, errno = f.Write([]byte("wazero"))
	require.EqualErrno(t, 0, errno)
	requireFileContent("wazero6789wazero")
}

func TestStdioFile_SetAppend(t *testing.T) {
	// SetAppend should not affect Stdio.
	file, err := NewStdioFile(false, os.Stdout)
	require.NoError(t, err)
	errno := file.SetAppend(true)
	require.EqualErrno(t, 0, errno)
	_, errno = file.Write([]byte{})
	require.EqualErrno(t, 0, errno)
}

func TestFileIno(t *testing.T) {
	tmpDir := t.TempDir()
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, tmpDir)

	// get the expected inode
	st, errno := stat(tmpDir)
	require.EqualErrno(t, 0, errno)

	tests := []struct {
		name        string
		fs          fs.FS
		expectedIno uint64
	}{
		{name: "os.DirFS", fs: dirFS, expectedIno: st.Ino},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			d, errno := OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer d.Close()

			ino, errno := d.Ino()
			require.EqualErrno(t, 0, errno)
			if !canReadDirInode() {
				tc.expectedIno = 0
			}
			require.Equal(t, tc.expectedIno, ino)
		})
	}

	t.Run("OS", func(t *testing.T) {
		d, errno := OpenOSFile(tmpDir, syscall.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer d.Close()

		ino, errno := d.Ino()
		require.EqualErrno(t, 0, errno)
		if canReadDirInode() {
			require.Equal(t, st.Ino, ino)
		} else {
			require.Zero(t, ino)
		}
	})
}

func canReadDirInode() bool {
	if runtime.GOOS != "windows" {
		return true
	} else {
		return strings.HasPrefix(runtime.Version(), "go1.20")
	}
}

func TestFileIsDir(t *testing.T) {
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, t.TempDir())

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Run("file", func(t *testing.T) {
				f, errno := OpenFSFile(tc.fs, wazeroFile, syscall.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				defer f.Close()

				isDir, errno := f.IsDir()
				require.EqualErrno(t, 0, errno)
				require.False(t, isDir)
			})

			t.Run("dir", func(t *testing.T) {
				d, errno := OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				defer d.Close()

				isDir, errno := d.IsDir()
				require.EqualErrno(t, 0, errno)
				require.True(t, isDir)
			})
		})
	}

	t.Run("OS dir", func(t *testing.T) {
		d, errno := OpenOSFile(t.TempDir(), syscall.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		defer d.Close()

		isDir, errno := d.IsDir()
		require.EqualErrno(t, 0, errno)
		require.True(t, isDir)
	})
}

func TestFileReadAndPread(t *testing.T) {
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, t.TempDir())

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, errno := OpenFSFile(tc.fs, wazeroFile, syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer f.Close()

			// The file should be readable (base case)
			requireRead(t, f, buf)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// We should be able to pread from zero also
			requirePread(t, f, buf, 0)
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// If the offset didn't change, read should expect the next three chars.
			requireRead(t, f, buf)
			require.Equal(t, "ero", string(buf))
			buf = buf[:]

			// We should also be able pread from any offset
			requirePread(t, f, buf, 2)
			require.Equal(t, "zer", string(buf))
		})
	}
}

func TestFilePollRead(t *testing.T) {
	// Test using os.Pipe as it is known to support poll.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	rF, err := NewStdioFile(true, r)
	require.NoError(t, err)
	buf := make([]byte, 10)
	timeout := time.Duration(0) // return immediately

	// When there's nothing in the pipe, it isn't ready.
	ready, errno := rF.PollRead(&timeout)
	if runtime.GOOS == "windows" {
		require.EqualErrno(t, syscall.ENOSYS, errno)
		t.Skip("TODO: windows File.PollRead")
	}
	require.EqualErrno(t, 0, errno)
	require.False(t, ready)

	// Write to the pipe to make the data available
	expected := []byte("wazero")
	_, err = w.Write([]byte("wazero"))
	require.NoError(t, err)

	// We should now be able to poll ready
	ready, errno = rF.PollRead(&timeout)
	require.EqualErrno(t, 0, errno)
	require.True(t, ready)

	// We should now be able to read from the pipe
	n, errno := rF.Read(buf)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(expected), n)
	require.Equal(t, expected, buf[:len(expected)])
}

func requireRead(t *testing.T, f fsapi.File, buf []byte) {
	n, errno := f.Read(buf)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func requirePread(t *testing.T, f fsapi.File, buf []byte, off int64) {
	n, errno := f.Pread(buf, off)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func TestFileRead_empty(t *testing.T) {
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, t.TempDir())

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, errno := OpenFSFile(tc.fs, emptyFile, syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer f.Close()

			t.Run("Read", func(t *testing.T) {
				// We should be able to read an empty file
				n, errno := f.Read(buf)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, n)
			})

			t.Run("Pread", func(t *testing.T) {
				n, errno := f.Pread(buf, 0)
				require.EqualErrno(t, 0, errno)
				require.Zero(t, n)
			})
		})
	}
}

type maskFS struct {
	fs.FS
}

func (m *maskFS) Open(name string) (fs.File, error) {
	f, err := m.FS.Open(name)
	return struct{ fs.File }{f}, err
}

func TestFilePread_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, errno := OpenFSFile(&maskFS{embedFS}, emptyFile, syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	defer f.Close()

	buf := make([]byte, 3)
	_, errno = f.Pread(buf, 0)
	require.EqualErrno(t, syscall.ENOSYS, errno)
}

func TestFileRead_Errors(t *testing.T) {
	// Create the file
	path := path.Join(t.TempDir(), emptyFile)

	// Open the file write-only
	flag := syscall.O_WRONLY | syscall.O_CREAT
	f := requireOpenFile(t, path, flag, 0o600)
	defer f.Close()
	buf := make([]byte, 5)

	tests := []struct {
		name string
		fn   func(fsapi.File) syscall.Errno
	}{
		{name: "Read", fn: func(f fsapi.File) syscall.Errno {
			_, errno := f.Read(buf)
			return errno
		}},
		{name: "Pread", fn: func(f fsapi.File) syscall.Errno {
			_, errno := f.Pread(buf, 0)
			return errno
		}},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Run("EBADF when not open for reading", func(t *testing.T) {
				// The descriptor exists, but not open for reading
				errno := tc.fn(f)
				require.EqualErrno(t, syscall.EBADF, errno)
			})
			testEISDIR(t, tc.fn)
		})
	}
}

func TestFileSeek(t *testing.T) {
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, t.TempDir())

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	buf := make([]byte, 3)

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, errno := OpenFSFile(tc.fs, wazeroFile, syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer f.Close()

			// Shouldn't be able to use an invalid whence
			_, errno = f.Seek(0, io.SeekEnd+1)
			require.EqualErrno(t, syscall.EINVAL, errno)
			_, errno = f.Seek(0, -1)
			require.EqualErrno(t, syscall.EINVAL, errno)

			// Shouldn't be able to seek before the file starts.
			_, errno = f.Seek(-1, io.SeekStart)
			require.EqualErrno(t, syscall.EINVAL, errno)

			requireRead(t, f, buf) // read 3 bytes

			// Seek to the start
			newOffset, errno := f.Seek(0, io.SeekStart)
			require.EqualErrno(t, 0, errno)

			// verify we can re-read from the beginning now.
			require.Zero(t, newOffset)
			requireRead(t, f, buf) // read 3 bytes again
			require.Equal(t, "waz", string(buf))
			buf = buf[:]

			// Seek to the start with zero allows you to read it back.
			newOffset, errno = f.Seek(0, io.SeekCurrent)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, int64(3), newOffset)

			// Seek to the last two bytes
			newOffset, errno = f.Seek(-2, io.SeekEnd)
			require.EqualErrno(t, 0, errno)

			// verify we can read the last two bytes
			require.Equal(t, int64(5), newOffset)
			n, errno := f.Read(buf)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, 2, n)
			require.Equal(t, "o\n", string(buf[:2]))

			t.Run("directory seek to zero", func(t *testing.T) {
				d, errno := OpenFSFile(tc.fs, ".", syscall.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				defer d.Close()

				_, errno = d.Seek(0, io.SeekStart)
				require.EqualErrno(t, 0, errno)
			})
		})
	}

	t.Run("os.File directory seek to zero", func(t *testing.T) {
		d := requireOpenFile(t, os.TempDir(), syscall.O_RDONLY|fsapi.O_DIRECTORY, 0o666)
		defer d.Close()

		_, errno := d.Seek(0, io.SeekStart)
		require.EqualErrno(t, 0, errno)
	})

	seekToZero := func(f fsapi.File) syscall.Errno {
		_, errno := f.Seek(0, io.SeekStart)
		return errno
	}
	testEBADFIfFileClosed(t, seekToZero)
}

func requireSeek(t *testing.T, f fsapi.File, off int64, whence int) int64 {
	n, errno := f.Seek(off, whence)
	require.EqualErrno(t, 0, errno)
	return n
}

func TestFileSeek_empty(t *testing.T) {
	dirFS, embedFS, mapFS := dirEmbedMapFS(t, t.TempDir())

	tests := []struct {
		name string
		fs   fs.FS
	}{
		{name: "os.DirFS", fs: dirFS},
		{name: "embed.api.FS", fs: embedFS},
		{name: "fstest.MapFS", fs: mapFS},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			f, errno := OpenFSFile(tc.fs, emptyFile, syscall.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer f.Close()

			t.Run("Start", func(t *testing.T) {
				require.Zero(t, requireSeek(t, f, 0, io.SeekStart))
			})

			t.Run("Current", func(t *testing.T) {
				require.Zero(t, requireSeek(t, f, 0, io.SeekCurrent))
			})

			t.Run("End", func(t *testing.T) {
				require.Zero(t, requireSeek(t, f, 0, io.SeekEnd))
			})
		})
	}
}

func TestFileSeek_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, errno := OpenFSFile(&maskFS{embedFS}, emptyFile, syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	defer f.Close()

	_, errno = f.Seek(0, io.SeekCurrent)
	require.EqualErrno(t, syscall.ENOSYS, errno)
}

func TestFileWriteAndPwrite(t *testing.T) {
	// fsapi.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	path := path.Join(t.TempDir(), wazeroFile)
	f := requireOpenFile(t, path, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	text := "wazero"
	buf := make([]byte, 3)
	copy(buf, text[:3])

	// The file should be writeable
	requireWrite(t, f, buf)

	// We should be able to pwrite at gap
	requirePwrite(t, f, buf, 6)

	copy(buf, text[3:])

	// If the offset didn't change, the next chars will write after the
	// first
	requireWrite(t, f, buf)

	// We should be able to pwrite the same bytes as above
	requirePwrite(t, f, buf, 9)

	// We should also be able to pwrite past the above.
	requirePwrite(t, f, buf, 12)

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	// We expect to have written the text two and a half times:
	//  1. Write: (file offset 0) "waz"
	//  2. Pwrite: offset 6 "waz"
	//  3. Write: (file offset 3) "ero"
	//  4. Pwrite: offset 9 "ero"
	//  4. Pwrite: offset 12 "ero"
	require.Equal(t, "wazerowazeroero", string(b))
}

func requireWrite(t *testing.T, f fsapi.File, buf []byte) {
	n, errno := f.Write(buf)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func requirePwrite(t *testing.T, f fsapi.File, buf []byte, off int64) {
	n, errno := f.Pwrite(buf, off)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, len(buf), n)
}

func TestFileWrite_empty(t *testing.T) {
	// fsapi.FS doesn't support writes, and there is no other built-in
	// implementation except os.File.
	path := path.Join(t.TempDir(), emptyFile)
	f := requireOpenFile(t, path, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	tests := []struct {
		name string
		fn   func(fsapi.File, []byte) (int, syscall.Errno)
	}{
		{name: "Write", fn: func(f fsapi.File, buf []byte) (int, syscall.Errno) {
			return f.Write(buf)
		}},
		{name: "Pwrite from zero", fn: func(f fsapi.File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 0)
		}},
		{name: "Pwrite from 3", fn: func(f fsapi.File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 3)
		}},
	}

	var emptyBuf []byte

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			n, errno := tc.fn(f, emptyBuf)
			require.EqualErrno(t, 0, errno)
			require.Zero(t, n)

			// The file should be empty
			b, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Zero(t, len(b))
		})
	}
}

func TestFileWrite_Unsupported(t *testing.T) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	// Use syscall.O_RDWR so that it fails due to type not flags
	f, errno := OpenFSFile(&maskFS{embedFS}, wazeroFile, syscall.O_RDWR, 0)
	require.EqualErrno(t, 0, errno)
	defer f.Close()

	tests := []struct {
		name string
		fn   func(fsapi.File, []byte) (int, syscall.Errno)
	}{
		{name: "Write", fn: func(f fsapi.File, buf []byte) (int, syscall.Errno) {
			return f.Write(buf)
		}},
		{name: "Pwrite", fn: func(f fsapi.File, buf []byte) (int, syscall.Errno) {
			return f.Pwrite(buf, 0)
		}},
	}

	buf := []byte("wazero")

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			_, errno := tc.fn(f, buf)
			require.EqualErrno(t, syscall.ENOSYS, errno)
		})
	}
}

func TestFileWrite_Errors(t *testing.T) {
	// Create the file
	path := path.Join(t.TempDir(), emptyFile)
	of, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, of.Close())

	// Open the file read-only
	flag := syscall.O_RDONLY
	f := requireOpenFile(t, path, flag, 0o600)
	defer f.Close()
	buf := []byte("wazero")

	tests := []struct {
		name string
		fn   func(fsapi.File) syscall.Errno
	}{
		{name: "Write", fn: func(f fsapi.File) syscall.Errno {
			_, errno := f.Write(buf)
			return errno
		}},
		{name: "Pwrite", fn: func(f fsapi.File) syscall.Errno {
			_, errno := f.Pwrite(buf, 0)
			return errno
		}},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Run("EBADF when not open for writing", func(t *testing.T) {
				// The descriptor exists, but not open for writing
				errno := tc.fn(f)
				require.EqualErrno(t, syscall.EBADF, errno)
			})
			testEISDIR(t, tc.fn)
		})
	}
}

func TestFileSync_NoError(t *testing.T) {
	testSync_NoError(t, fsapi.File.Sync)
}

func TestFileDatasync_NoError(t *testing.T) {
	testSync_NoError(t, fsapi.File.Datasync)
}

func testSync_NoError(t *testing.T, sync func(fsapi.File) syscall.Errno) {
	roPath := "file_test.go"
	ro, errno := OpenFSFile(embedFS, roPath, syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	defer ro.Close()

	rwPath := path.Join(t.TempDir(), "datasync")
	rw, errno := OpenOSFile(rwPath, syscall.O_CREAT|syscall.O_RDWR, 0o600)
	require.EqualErrno(t, 0, errno)
	defer rw.Close()

	tests := []struct {
		name string
		f    fsapi.File
	}{
		{name: "UnimplementedFile", f: fsapi.UnimplementedFile{}},
		{name: "File of read-only fs.File", f: ro},
		{name: "File of os.File", f: rw},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.EqualErrno(t, 0, sync(tc.f))
		})
	}
}

func TestFileSync(t *testing.T) {
	testSync(t, fsapi.File.Sync)
}

func TestFileDatasync(t *testing.T) {
	testSync(t, fsapi.File.Datasync)
}

// testSync doesn't guarantee sync works because the operating system may
// sync anyway. There is no test in Go for syscall.Fdatasync, but closest is
// similar to below. Effectively, this only tests that things don't error.
func testSync(t *testing.T, sync func(fsapi.File) syscall.Errno) {
	// Even though it is invalid, try to sync a directory
	dPath := t.TempDir()
	d := requireOpenFile(t, dPath, syscall.O_RDONLY, 0)
	defer d.Close()

	errno := sync(d)
	require.EqualErrno(t, 0, errno)

	fPath := path.Join(dPath, t.Name())

	f := requireOpenFile(t, fPath, syscall.O_RDWR|os.O_CREATE, 0o600)
	defer f.Close()

	expected := "hello world!"

	// Write the expected data
	_, errno = f.Write([]byte(expected))
	require.EqualErrno(t, 0, errno)

	// Sync the data.
	errno = sync(f)
	require.EqualErrno(t, 0, errno)

	// Rewind while the file is still open.
	_, errno = f.Seek(0, io.SeekStart)
	require.EqualErrno(t, 0, errno)

	// Read data from the file
	buf := make([]byte, 50)
	n, errno := f.Read(buf)
	require.EqualErrno(t, 0, errno)

	// It may be the case that sync worked.
	require.Equal(t, expected, string(buf[:n]))

	// Windows allows you to sync a closed file
	if runtime.GOOS != "windows" {
		testEBADFIfFileClosed(t, sync)
		testEBADFIfDirClosed(t, sync)
	}
}

func TestFileTruncate(t *testing.T) {
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

			fPath := path.Join(tmpDir, tc.name)
			f := openForWrite(t, fPath, content)
			defer f.Close()

			errno := f.Truncate(tc.size)
			require.EqualErrno(t, 0, errno)

			actual, err := os.ReadFile(fPath)
			require.NoError(t, err)
			require.Equal(t, tc.expectedContent, actual)
		})
	}

	truncateToZero := func(f fsapi.File) syscall.Errno {
		return f.Truncate(0)
	}

	if runtime.GOOS != "windows" {
		// TODO: os.Truncate on windows passes even when closed
		testEBADFIfFileClosed(t, truncateToZero)
	}

	testEISDIR(t, truncateToZero)

	t.Run("negative", func(t *testing.T) {
		tmpDir := t.TempDir()

		f := openForWrite(t, path.Join(tmpDir, "truncate"), content)
		defer f.Close()

		errno := f.Truncate(-1)
		require.EqualErrno(t, syscall.EINVAL, errno)
	})
}

func TestFileUtimens(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin": // supported
	case "freebsd": // TODO: support freebsd w/o CGO
	case "windows":
		if !platform.IsGo120 {
			t.Skip("windows only works after Go 1.20") // TODO: possibly 1.19 ;)
		}
	default: // expect ENOSYS and callers need to fall back to Utimens
		t.Skip("unsupported GOOS", runtime.GOOS)
	}

	testUtimens(t, true)

	testEBADFIfFileClosed(t, func(f fsapi.File) syscall.Errno {
		return f.Utimens(nil)
	})
	testEBADFIfDirClosed(t, func(d fsapi.File) syscall.Errno {
		return d.Utimens(nil)
	})
}

func TestNewStdioFile(t *testing.T) {
	// simulate regular file attached to stdin
	f, err := os.CreateTemp(t.TempDir(), "somefile")
	require.NoError(t, err)
	defer f.Close()

	stdin, err := NewStdioFile(true, os.Stdin)
	require.NoError(t, err)
	stdinStat, err := os.Stdin.Stat()
	require.NoError(t, err)

	stdinFile, err := NewStdioFile(true, f)
	require.NoError(t, err)

	stdout, err := NewStdioFile(false, os.Stdout)
	require.NoError(t, err)
	stdoutStat, err := os.Stdout.Stat()
	require.NoError(t, err)

	stdoutFile, err := NewStdioFile(false, f)
	require.NoError(t, err)

	tests := []struct {
		name string
		f    fsapi.File
		// Depending on how the tests run, os.Stdin won't necessarily be a char
		// device. We compare against an os.File, to account for this.
		expectedType fs.FileMode
	}{
		{
			name:         "stdin",
			f:            stdin,
			expectedType: stdinStat.Mode().Type(),
		},
		{
			name:         "stdin file",
			f:            stdinFile,
			expectedType: 0, // normal file
		},
		{
			name:         "stdout",
			f:            stdout,
			expectedType: stdoutStat.Mode().Type(),
		},
		{
			name:         "stdout file",
			f:            stdoutFile,
			expectedType: 0, // normal file
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name+" Stat", func(t *testing.T) {
			st, errno := tc.f.Stat()
			require.EqualErrno(t, 0, errno)
			require.Equal(t, tc.expectedType, st.Mode&fs.ModeType)
			require.Equal(t, uint64(1), st.Nlink)

			// Fake times are needed to pass wasi-testsuite.
			// See https://github.com/WebAssembly/wasi-testsuite/blob/af57727/tests/rust/src/bin/fd_filestat_get.rs#L1-L19
			require.Zero(t, st.Ctim)
			require.Zero(t, st.Mtim)
			require.Zero(t, st.Atim)
		})
	}
}

func testEBADFIfDirClosed(t *testing.T, fn func(fsapi.File) syscall.Errno) bool {
	return t.Run("EBADF if dir closed", func(t *testing.T) {
		d := requireOpenFile(t, t.TempDir(), syscall.O_RDONLY, 0o755)

		// close the directory underneath
		require.EqualErrno(t, 0, d.Close())

		require.EqualErrno(t, syscall.EBADF, fn(d))
	})
}

func testEBADFIfFileClosed(t *testing.T, fn func(fsapi.File) syscall.Errno) bool {
	return t.Run("EBADF if file closed", func(t *testing.T) {
		tmpDir := t.TempDir()

		f := openForWrite(t, path.Join(tmpDir, "EBADF"), []byte{1, 2, 3, 4})

		// close the file underneath
		require.EqualErrno(t, 0, f.Close())

		require.EqualErrno(t, syscall.EBADF, fn(f))
	})
}

func testEISDIR(t *testing.T, fn func(fsapi.File) syscall.Errno) bool {
	return t.Run("EISDIR if directory", func(t *testing.T) {
		f := requireOpenFile(t, os.TempDir(), syscall.O_RDONLY|fsapi.O_DIRECTORY, 0o666)
		defer f.Close()

		require.EqualErrno(t, syscall.EISDIR, fn(f))
	})
}

func openForWrite(t *testing.T, path string, content []byte) fsapi.File {
	require.NoError(t, os.WriteFile(path, content, 0o0666))
	f := requireOpenFile(t, path, syscall.O_RDWR, 0o666)
	_, errno := f.Write(content)
	require.EqualErrno(t, 0, errno)
	return f
}

func requireOpenFile(t *testing.T, path string, flag int, perm fs.FileMode) fsapi.File {
	f, errno := OpenOSFile(path, flag, perm)
	require.EqualErrno(t, 0, errno)
	return f
}

func dirEmbedMapFS(t *testing.T, tmpDir string) (fs.FS, fs.FS, fs.FS) {
	embedFS, err := fs.Sub(testdata, "testdata")
	require.NoError(t, err)

	f, err := embedFS.Open(wazeroFile)
	require.NoError(t, err)
	defer f.Close()

	bytes, err := io.ReadAll(f)
	require.NoError(t, err)

	mapFS := gofstest.MapFS{
		emptyFile:  &gofstest.MapFile{},
		wazeroFile: &gofstest.MapFile{Data: bytes},
	}

	// Write a file as can't open "testdata" in scratch tests because they
	// can't read the original filesystem.
	require.NoError(t, os.WriteFile(path.Join(tmpDir, emptyFile), nil, 0o600))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, wazeroFile), bytes, 0o600))
	dirFS := os.DirFS(tmpDir)
	return dirFS, embedFS, mapFS
}

func TestReaddirStructs(t *testing.T) {
	makeDirents := func(begin, end int) []fsapi.Dirent {
		dirents := make([]fsapi.Dirent, end-begin)
		for i := begin; i < end; i++ {
			dirents[i-begin] = fsapi.Dirent{
				Name: fmt.Sprintf("f_%d.dat", i),
			}
		}
		return dirents
	}

	tempDir := t.TempDir()
	numFiles := 3*direntBufSize + 5
	for i := 0; i < numFiles; i++ {
		fname := fmt.Sprintf("f_%d.dat", i)
		fullname := path.Join(tempDir, fname)
		err := os.WriteFile(fullname, []byte(fname), 0o0644)
		if err != nil {
			panic(err)
		}
	}

	cons := []struct {
		name         string
		expectedSize int
		newReaddir   func() fsapi.Readdir
	}{
		{
			name:       "emptyReaddir",
			newReaddir: func() fsapi.Readdir { return emptyReaddir{} },
		},

		{
			name:         "sliceReaddir",
			expectedSize: 4,
			newReaddir: func() fsapi.Readdir {
				return NewReaddir(makeDirents(0, 4)...)
			},
		},
		{
			name:         "concatReaddir",
			expectedSize: 8,
			newReaddir: func() fsapi.Readdir {
				return NewConcatReaddir(
					NewReaddir(makeDirents(0, 4)...),
					NewReaddir(makeDirents(4, 8)...))
			},
		},
		{
			name:         "windowedReaddir",
			expectedSize: direntBufSize*2 + 3,
			newReaddir: func() fsapi.Readdir {
				var count uint64
				var entriesLeft uint64

				readdir, errno := newWindowedReaddir(
					func() syscall.Errno {
						count = 0
						entriesLeft = direntBufSize*2 + 3
						return 0
					},
					// Emit windows of at most n elements, until the expectedSize is reached.
					func(n uint64) (fsapi.Readdir, syscall.Errno) {
						if entriesLeft > n {
							entriesLeft -= n
						} else if entriesLeft == 0 {
							return nil, syscall.ENOENT
						} else {
							n = entriesLeft
							entriesLeft = 0
						}
						readdir := NewReaddir(makeDirents(int(count), int(count+n))...)
						count += n
						return readdir, 0
					},
					func() syscall.Errno { return 0 })
				require.EqualErrno(t, 0, errno)
				return readdir
			},
		},
		{
			name:         "windowedReaddir (from file)",
			expectedSize: numFiles,
			newReaddir: func() fsapi.Readdir {
				file, errno := OpenOSFile(tempDir, syscall.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				readdir, errno := newReaddirFromFile(file.(rawOsFile), tempDir)
				require.EqualErrno(t, 0, errno)
				return readdir
			},
		},
	}

	tests := []struct {
		name string
		f    func(t *testing.T, r fsapi.Readdir, size int)
	}{
		{
			name: "read 1",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				d, errno := r.Next()
				if size == 0 {
					require.EqualErrno(t, syscall.ENOENT, errno)
					return
				}
				require.EqualErrno(t, 0, errno)
				require.NotNil(t, d)
			},
		},
		{
			name: "read 2",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				d, errno := r.Next()
				if size == 0 {
					require.EqualErrno(t, syscall.ENOENT, errno)
					return
				}
				require.EqualErrno(t, 0, errno)
				require.NotNil(t, d)

				d, errno = r.Next()
				require.EqualErrno(t, 0, errno)
				require.NotNil(t, d)
			},
		},
		{
			name: "read half",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				var errno syscall.Errno
				for i := 0; i < size/2; i++ {
					_, errno = r.Next()
				}
				require.EqualErrno(t, 0, errno)
			},
		},
		{
			name: "read all",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				dirents, errno := ReaddirAll(r)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, size, len(dirents))
			},
		},
		{
			name: "exhausted returns ENOENT",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				dirents, errno := ReaddirAll(r)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, size, len(dirents))
				_, errno = r.Next()
				require.EqualErrno(t, syscall.ENOENT, errno)
			},
		},
		{
			name: "exhausted can be Reset() and exhausted again",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				dirents, errno := ReaddirAll(r)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, size, len(dirents))
				_, errno = r.Next()
				require.EqualErrno(t, syscall.ENOENT, errno)

				errno = r.Rewind(0)
				require.EqualErrno(t, 0, errno)

				dirents, errno = ReaddirAll(r)
				require.EqualErrno(t, 0, errno)
				require.Equal(t, size, len(dirents))
				_, errno = r.Next()
				require.EqualErrno(t, syscall.ENOENT, errno)
			},
		},
		{
			name: "Offset() is always increasing",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				count := uint64(1)
				for {
					if _, errno := r.Next(); errno == syscall.ENOENT {
						break
					}
					require.Equal(t, count, r.Offset())
					count++
				}
			},
		},
		{
			name: "Rewind() within a window",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				var errno syscall.Errno
				half := size / 2
				for i := 0; i < half; i++ {
					_, errno = r.Next()
				}
				require.EqualErrno(t, 0, errno)
				// Rewind to the start of the current window.
				nwindow := uint64(half / direntBufSize)
				errno = r.Rewind(nwindow * direntBufSize)
				require.EqualErrno(t, 0, errno)
			},
		},
		{
			name: "Rewind(Offset()-1) is always valid with Offset()>0",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				count := uint64(0)
				var last *fsapi.Dirent
				for {
					curr, errno := r.Next()
					last = curr
					if errno == syscall.ENOENT {
						break
					}
					count++

					require.Equal(t, count, r.Offset())

					errno = r.Rewind(r.Offset() - 1)
					require.EqualErrno(t, 0, errno, fmt.Sprintf("failed at index %d", r.Offset()-1))

					curr, errno = r.Next()
					require.Equal(t, last.Name, curr.Name)
					require.EqualErrno(t, 0, errno)
				}
			},
		},
		{
			name: "Cannot Rewind() to the last element of a previous window",
			f: func(t *testing.T, r fsapi.Readdir, size int) {
				half := size / 2
				for i := 0; i < half; i++ {
					_, errno := r.Next()
					require.EqualErrno(t, 0, errno)
				}
				// Rewind to the start of the current window.
				nwindow := uint64(half / direntBufSize)
				if nwindow != 0 {
					idx := nwindow*direntBufSize - 1
					errno := r.Rewind(idx)
					require.EqualErrno(t, syscall.ENOSYS, errno, fmt.Sprintf("failed at index %d", idx))
				}
			},
		},
	}

	for _, c := range cons {
		t.Run(c.name, func(t *testing.T) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					d := c.newReaddir()
					defer func() {
						if errno := d.Close(); errno != 0 {
							panic(errno)
						}
					}()
					tc.f(t, d, c.expectedSize)
				})
			}
		})
	}

	t.Run("rewind concat dot-dirs + windowed readdir from file", func(t *testing.T) {
		file, errno := OpenOSFile(tempDir, syscall.O_RDONLY, 0)
		require.EqualErrno(t, 0, errno)
		dotEntries := NewReaddir(fsapi.Dirent{Name: "."}, fsapi.Dirent{Name: ".."})
		readdirF, errno := newReaddirFromFile(file.(rawOsFile), tempDir)

		require.EqualErrno(t, 0, errno)
		readdir := NewConcatReaddir(dotEntries, readdirF)

		dot, _ := readdir.Next()
		require.NotNil(t, dot)
		dotdot, _ := readdir.Next()
		require.NotNil(t, dotdot)

		// Current item under the cursor is offset 2.
		require.Equal(t, uint64(2), readdir.Offset())
		// Read the first item of readdirF and advance the internal cursor.
		first, _ := readdir.Next()
		require.NotNil(t, first)
		require.Equal(t, uint64(3), readdir.Offset())

		// get back to the previous iterator in the concat (dotEntries)
		errno = readdir.Rewind(1)
		require.EqualErrno(t, 0, errno)
		dotdot2, errno := readdir.Next()
		require.NotNil(t, dotdot2)
		require.Equal(t, dotdot, dotdot2)
		require.EqualErrno(t, 0, errno)

		// Read again the first item of readdirF and advance the internal cursor.
		first2, _ := readdir.Next()
		require.NotNil(t, first2)
		require.Equal(t, first, first2)
		require.Equal(t, uint64(3), readdir.Offset())
	})
}

func TestReaddDir_Rewind(t *testing.T) {
	tests := []struct {
		name           string
		f              fsapi.Readdir
		offset         uint64
		expectedCookie int64
		expectedErrno  syscall.Errno
	}{
		{
			name: "no prior call",
		},
		{
			name:          "no prior call, but passed a cookie",
			offset:        1,
			expectedErrno: syscall.EINVAL,
		},
		{
			name: "cookie is greater than last d_next",
			f: &windowedReaddir{
				cursor: 3,
				window: emptyReaddir{},
			},
			offset:        5,
			expectedErrno: syscall.EINVAL,
		},
		{
			name: "cookie is last pos",
			f: &windowedReaddir{
				cursor: 3,
				window: emptyReaddir{},
			},
			offset: 3,
		},
		{
			name: "cookie is one before last pos",
			f: &windowedReaddir{
				cursor: 3,
				window: emptyReaddir{},
			},
			offset: 2,
		},
		{
			name: "cookie is before current entries",
			f: &windowedReaddir{
				cursor: direntBufSize + 2,
				window: emptyReaddir{},
			},
			offset:        1,
			expectedErrno: syscall.ENOSYS, // not implemented
		},
		{
			name: "read from the beginning (cookie=0)",
			f: &windowedReaddir{
				init: func() syscall.Errno { return 0 },
				fetch: func(n uint64) (fsapi.Readdir, syscall.Errno) {
					return NewReaddir(fsapi.Dirent{Name: "."}, fsapi.Dirent{Name: ".."}), 0
				},
				cursor: 3,
				window: emptyReaddir{},
			},
			offset: 0,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if f == nil {
				f = &windowedReaddir{
					init:   func() syscall.Errno { return 0 },
					fetch:  func(n uint64) (fsapi.Readdir, syscall.Errno) { return nil, 0 },
					window: emptyReaddir{},
				}
			}

			errno := f.Rewind(tc.offset)
			require.EqualErrno(t, tc.expectedErrno, errno)
		})
	}
}
