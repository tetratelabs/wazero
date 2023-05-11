package sys

import (
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/internal/descriptor"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sysfs"
)

const (
	FdStdin int32 = iota
	FdStdout
	FdStderr
	// FdPreopen is the file descriptor of the first pre-opened directory.
	//
	// # Why file descriptor 3?
	//
	// While not specified, the most common WASI implementation, wasi-libc,
	// expects POSIX style file descriptor allocation, where the lowest
	// available number is used to open the next file. Since 1 and 2 are taken
	// by stdout and stderr, the next is 3.
	//   - https://github.com/WebAssembly/WASI/issues/122
	//   - https://pubs.opengroup.org/onlinepubs/9699919799/functions/V2_chap02.html#tag_15_14
	//   - https://github.com/WebAssembly/wasi-libc/blob/wasi-sdk-16/libc-bottom-half/sources/preopens.c#L215
	FdPreopen
)

const modeDevice = fs.ModeDevice | 0o640

// StdinFile is a fs.ModeDevice file for use implementing FdStdin.
// This is safer than reading from os.DevNull as it can never overrun
// operating system file descriptors.
type StdinFile struct {
	noopStdinFile
	io.Reader
}

// Read implements the same method as documented on platform.File
func (f *StdinFile) Read(buf []byte) (int, syscall.Errno) {
	n, err := f.Reader.Read(buf)
	return n, platform.UnwrapOSError(err)
}

type writerFile struct {
	noopStdoutFile

	w io.Writer
}

// Write implements the same method as documented on platform.File
func (f *writerFile) Write(buf []byte) (int, syscall.Errno) {
	n, err := f.w.Write(buf)
	return n, platform.UnwrapOSError(err)
}

// noopStdinFile is a fs.ModeDevice file for use implementing FdStdin. This is
// safer than reading from os.DevNull as it can never overrun operating system
// file descriptors.
type noopStdinFile struct {
	noopStdioFile
}

// AccessMode implements the same method as documented on platform.File
func (noopStdinFile) AccessMode() int {
	return syscall.O_RDONLY
}

// Read implements the same method as documented on platform.File
func (noopStdinFile) Read([]byte) (int, syscall.Errno) {
	return 0, 0 // Always EOF
}

// PollRead implements the same method as documented on platform.File
func (noopStdinFile) PollRead(*time.Duration) (ready bool, errno syscall.Errno) {
	return true, 0 // always ready to read nothing
}

// noopStdoutFile is a fs.ModeDevice file for use implementing FdStdout and
// FdStderr.
type noopStdoutFile struct {
	noopStdioFile
}

// AccessMode implements the same method as documented on platform.File
func (noopStdoutFile) AccessMode() int {
	return syscall.O_WRONLY
}

// Write implements the same method as documented on platform.File
func (noopStdoutFile) Write(buf []byte) (int, syscall.Errno) {
	return len(buf), 0 // same as io.Discard
}

type noopStdioFile struct {
	platform.UnimplementedFile
}

// Stat implements the same method as documented on platform.File
func (noopStdioFile) Stat() (platform.Stat_t, syscall.Errno) {
	return platform.Stat_t{Mode: modeDevice, Nlink: 1}, 0
}

// IsDir implements the same method as documented on platform.File
func (noopStdioFile) IsDir() (bool, syscall.Errno) {
	return false, 0
}

// Close implements the same method as documented on platform.File
func (noopStdioFile) Close() (errno syscall.Errno) { return }

// compile-time check to ensure lazyDir implements platform.File.
var _ platform.File = (*lazyDir)(nil)

type lazyDir struct {
	platform.DirFile

	fs sysfs.FS
	f  platform.File
}

// Ino implements the same method as documented on platform.File
func (r *lazyDir) Ino() (uint64, syscall.Errno) {
	if f, ok := r.file(); !ok {
		return 0, syscall.EBADF
	} else {
		return f.Ino()
	}
}

// IsAppend implements the same method as documented on platform.File
func (r *lazyDir) IsAppend() bool {
	return false
}

// SetAppend implements the same method as documented on platform.File
func (r *lazyDir) SetAppend(bool) syscall.Errno {
	return syscall.EISDIR
}

// Seek implements the same method as documented on platform.File
func (r *lazyDir) Seek(offset int64, whence int) (newOffset int64, errno syscall.Errno) {
	if f, ok := r.file(); !ok {
		return 0, syscall.EBADF
	} else {
		return f.Seek(offset, whence)
	}
}

// Stat implements the same method as documented on platform.File
func (r *lazyDir) Stat() (platform.Stat_t, syscall.Errno) {
	if f, ok := r.file(); !ok {
		return platform.Stat_t{}, syscall.EBADF
	} else {
		return f.Stat()
	}
}

// Readdir implements the same method as documented on platform.File
func (r *lazyDir) Readdir(n int) (dirents []platform.Dirent, errno syscall.Errno) {
	if f, ok := r.file(); !ok {
		return nil, syscall.EBADF
	} else {
		return f.Readdir(n)
	}
}

// Sync implements the same method as documented on platform.File
func (r *lazyDir) Sync() syscall.Errno {
	if f, ok := r.file(); !ok {
		return syscall.EBADF
	} else {
		return f.Sync()
	}
}

// Datasync implements the same method as documented on platform.File
func (r *lazyDir) Datasync() syscall.Errno {
	if f, ok := r.file(); !ok {
		return syscall.EBADF
	} else {
		return f.Datasync()
	}
}

// Chmod implements the same method as documented on platform.File
func (r *lazyDir) Chmod(mode fs.FileMode) syscall.Errno {
	if f, ok := r.file(); !ok {
		return syscall.EBADF
	} else {
		return f.Chmod(mode)
	}
}

// Chown implements the same method as documented on platform.File
func (r *lazyDir) Chown(uid, gid int) syscall.Errno {
	if f, ok := r.file(); !ok {
		return syscall.EBADF
	} else {
		return f.Chown(uid, gid)
	}
}

// Utimens implements the same method as documented on platform.File
func (r *lazyDir) Utimens(times *[2]syscall.Timespec) syscall.Errno {
	if f, ok := r.file(); !ok {
		return syscall.EBADF
	} else {
		return f.Utimens(times)
	}
}

// file returns the underlying file or false if it doesn't exist.
func (r *lazyDir) file() (platform.File, bool) {
	if f := r.f; r.f != nil {
		return f, true
	}
	var errno syscall.Errno
	r.f, errno = r.fs.OpenFile(".", os.O_RDONLY, 0)
	switch errno {
	case 0:
		return r.f, true
	case syscall.ENOENT:
		return nil, false
	default:
		panic(errno) // unexpected
	}
}

// Close implements fs.File
func (r *lazyDir) Close() syscall.Errno {
	f := r.f
	if f == nil {
		return 0 // never opened
	}
	return f.Close()
}

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	// Name is the name of the directory up to its pre-open, or the pre-open
	// name itself when IsPreopen.
	//
	// # Notes
	//
	//   - This can drift on rename.
	//   - This relates to the guest path, which is not the real file path
	//     except if the entire host filesystem was made available.
	Name string

	// IsPreopen is a directory that is lazily opened.
	IsPreopen bool

	// FS is the filesystem associated with the pre-open.
	FS sysfs.FS

	// File is always non-nil.
	File platform.File

	// ReadDir is present when this File is a fs.ReadDirFile and `ReadDir`
	// was called.
	ReadDir *ReadDir
}

// ReadDir is the status of a prior fs.ReadDirFile call.
type ReadDir struct {
	// CountRead is the total count of files read including Dirents.
	CountRead uint64

	// Dirents is the contents of the last platform.Readdir call. Notably,
	// directory listing are not rewindable, so we keep entries around in case
	// the caller mis-estimated their buffer and needs a few still cached.
	//
	// Note: This is wasi-specific and needs to be refactored.
	// In wasi preview1, dot and dot-dot entries are required to exist, but the
	// reverse is true for preview2. More importantly, preview2 holds separate
	// stateful dir-entry-streams per file.
	Dirents []platform.Dirent
}

type FSContext struct {
	// rootFS is the root ("/") mount.
	rootFS sysfs.FS

	// openedFiles is a map of file descriptor numbers (>=FdPreopen) to open files
	// (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles FileTable
}

// FileTable is a specialization of the descriptor.Table type used to map file
// descriptors to file entries.
type FileTable = descriptor.Table[int32, *FileEntry]

// NewFSContext creates a FSContext with stdio streams and an optional
// pre-opened filesystem.
//
// If `preopened` is not sysfs.UnimplementedFS, it is inserted into
// the file descriptor table as FdPreopen.
func (c *Context) NewFSContext(stdin io.Reader, stdout, stderr io.Writer, rootFS sysfs.FS) (err error) {
	c.fsc.rootFS = rootFS
	inFile, err := stdinFile(stdin)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(inFile)
	outWriter, err := stdioWriterFile("stdout", stdout)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(outWriter)
	errWriter, err := stdioWriterFile("stderr", stderr)
	if err != nil {
		return err
	}
	c.fsc.openedFiles.Insert(errWriter)

	if _, ok := rootFS.(sysfs.UnimplementedFS); ok {
		return nil
	}

	if comp, ok := rootFS.(*sysfs.CompositeFS); ok {
		preopens := comp.FS()
		for i, p := range comp.GuestPaths() {
			c.fsc.openedFiles.Insert(&FileEntry{
				FS:        preopens[i],
				Name:      p,
				IsPreopen: true,
				File:      &lazyDir{fs: rootFS},
			})
		}
	} else {
		c.fsc.openedFiles.Insert(&FileEntry{
			FS:        rootFS,
			Name:      "/",
			IsPreopen: true,
			File:      &lazyDir{fs: rootFS},
		})
	}

	return nil
}

func stdinFile(r io.Reader) (*FileEntry, error) {
	if r == nil {
		return &FileEntry{Name: "stdin", IsPreopen: true, File: &noopStdinFile{}}, nil
	} else if f, ok := r.(*os.File); ok {
		if f, err := platform.NewStdioFile(true, f); err != nil {
			return nil, err
		} else {
			return &FileEntry{Name: "stdin", IsPreopen: true, File: f}, nil
		}
	} else {
		return &FileEntry{Name: "stdin", IsPreopen: true, File: &StdinFile{Reader: r}}, nil
	}
}

func stdioWriterFile(name string, w io.Writer) (*FileEntry, error) {
	if w == nil {
		return &FileEntry{Name: name, IsPreopen: true, File: &noopStdoutFile{}}, nil
	} else if f, ok := w.(*os.File); ok {
		if f, err := platform.NewStdioFile(false, f); err != nil {
			return nil, err
		} else {
			return &FileEntry{Name: name, IsPreopen: true, File: f}, nil
		}
	} else {
		return &FileEntry{Name: name, IsPreopen: true, File: &writerFile{w: w}}, nil
	}
}

// RootFS returns the underlying filesystem. Any files that should be added to
// the table should be inserted via InsertFile.
func (c *FSContext) RootFS() sysfs.FS {
	return c.rootFS
}

// OpenFile opens the file into the table and returns its file descriptor.
// The result must be closed by CloseFile or Close.
func (c *FSContext) OpenFile(fs sysfs.FS, path string, flag int, perm fs.FileMode) (int32, syscall.Errno) {
	if f, errno := fs.OpenFile(path, flag, perm); errno != 0 {
		return 0, errno
	} else {
		fe := &FileEntry{FS: fs, File: f}
		if path == "/" || path == "." {
			fe.Name = ""
		} else {
			fe.Name = path
		}
		if newFD, ok := c.openedFiles.Insert(fe); !ok {
			return 0, syscall.EBADF
		} else {
			return newFD, 0
		}
	}
}

// LookupFile returns a file if it is in the table.
func (c *FSContext) LookupFile(fd int32) (*FileEntry, bool) {
	return c.openedFiles.Lookup(fd)
}

// Renumber assigns the file pointed by the descriptor `from` to `to`.
func (c *FSContext) Renumber(from, to int32) syscall.Errno {
	fromFile, ok := c.openedFiles.Lookup(from)
	if !ok || to < 0 {
		return syscall.EBADF
	} else if fromFile.IsPreopen {
		return syscall.ENOTSUP
	}

	// If toFile is already open, we close it to prevent windows lock issues.
	//
	// The doc is unclear and other implementations do nothing for already-opened To FDs.
	// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_renumberfd-fd-to-fd---errno
	// https://github.com/bytecodealliance/wasmtime/blob/main/crates/wasi-common/src/snapshots/preview_1.rs#L531-L546
	if toFile, ok := c.openedFiles.Lookup(to); ok {
		if toFile.IsPreopen {
			return syscall.ENOTSUP
		}
		_ = toFile.File.Close()
	}

	c.openedFiles.Delete(from)
	if !c.openedFiles.InsertAt(fromFile, to) {
		return syscall.EBADF
	}
	return 0
}

// CloseFile returns any error closing the existing file.
func (c *FSContext) CloseFile(fd int32) syscall.Errno {
	f, ok := c.openedFiles.Lookup(fd)
	if !ok {
		return syscall.EBADF
	}
	c.openedFiles.Delete(fd)
	return platform.UnwrapOSError(f.File.Close())
}

// Close implements io.Closer
func (c *FSContext) Close() (err error) {
	// Close any files opened in this context
	c.openedFiles.Range(func(fd int32, entry *FileEntry) bool {
		if errno := entry.File.Close(); errno != 0 {
			err = errno // This means err returned == the last non-nil error.
		}
		return true
	})
	// A closed FSContext cannot be reused so clear the state instead of
	// using Reset.
	c.openedFiles = FileTable{}
	return
}
