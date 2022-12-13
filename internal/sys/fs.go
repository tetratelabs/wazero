package sys

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	FdStdin uint32 = iota
	FdStdout
	FdStderr

	// FdRoot is the file descriptor of the root ("/") filesystem.
	//
	// # Why file descriptor 3?
	//
	// While not specified, the most common WASI implementation, wasi-libc, expects
	// POSIX style file descriptor allocation, where the lowest available number is
	// used to open the next file. Since 1 and 2 are taken by stdout and stderr,
	// `root` is assigned 3.
	//   - https://github.com/WebAssembly/WASI/issues/122
	//   - https://pubs.opengroup.org/onlinepubs/9699919799/functions/V2_chap02.html#tag_15_14
	//   - https://github.com/WebAssembly/wasi-libc/blob/wasi-sdk-16/libc-bottom-half/sources/preopens.c#L215
	FdRoot
)

// FSKey is a context.Context Value key. It allows overriding fs.FS for WASI.
//
// See https://github.com/tetratelabs/wazero/issues/491
type FSKey struct{}

// EmptyFS is exported to special-case an empty file system.
var EmptyFS = &emptyFS{}

type emptyFS struct{}

// compile-time check to ensure emptyFS implements fs.FS
var _ fs.FS = &emptyFS{}

// Open implements the same method as documented on fs.FS.
func (f *emptyFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// An emptyRootDir is a fake "/" directory.
type emptyRootDir struct{}

var _ fs.ReadDirFile = emptyRootDir{}

func (emptyRootDir) Stat() (fs.FileInfo, error) { return emptyRootDir{}, nil }
func (emptyRootDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: "/", Err: errors.New("is a directory")}
}
func (emptyRootDir) Close() error                       { return nil }
func (emptyRootDir) ReadDir(int) ([]fs.DirEntry, error) { return nil, nil }

var _ fs.FileInfo = emptyRootDir{}

func (emptyRootDir) Name() string       { return "/" }
func (emptyRootDir) Size() int64        { return 0 }
func (emptyRootDir) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (emptyRootDir) ModTime() time.Time { return time.Unix(0, 0) }
func (emptyRootDir) IsDir() bool        { return true }
func (emptyRootDir) Sys() interface{}   { return nil }

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	// Path was the argument to FSContext.OpenFile
	// TODO: we may need an additional field which is the full path.
	Path string

	// File is always non-nil, even when root "/" (fd=FdRoot)
	File fs.File

	// ReadDir is present when this File is a fs.ReadDirFile and `ReadDir`
	// was called.
	ReadDir *ReadDir
}

// ReadDir is the status of a prior fs.ReadDirFile call.
type ReadDir struct {
	// CountRead is the total count of files read including Entries.
	CountRead uint64

	// Entries is the contents of the last fs.ReadDirFile call. Notably,
	// directory listing are not rewindable, so we keep entries around in case
	// the caller mis-estimated their buffer and needs a few still cached.
	Entries []fs.DirEntry
}

type FSContext struct {
	// fs is the root ("/") mount.
	fs fs.FS

	// openedFiles is a map of file descriptor numbers (>=FdRoot) to open files
	// (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles map[uint32]*FileEntry

	// lastFD is not meant to be read directly. Rather by nextFD.
	lastFD uint32
}

// emptyFSContext is the context associated with EmptyFS.
//
// Note: This is not mutable as operations functions do not affect field state.
var emptyFSContext = &FSContext{
	fs:          EmptyFS,
	openedFiles: map[uint32]*FileEntry{},
	lastFD:      2,
}

// NewFSContext creates a FSContext, using the `root` parameter for any paths
// beginning at "/". If the input is EmptyFS, there is no root filesystem.
// Otherwise, `root` is assigned file descriptor FdRoot and the returned
// context can open files in that file system.
//
// If root is a fs.ReadDirFS, any error on opening "." is returned.
func NewFSContext(root fs.FS) (fsc *FSContext, err error) {
	if root == EmptyFS {
		fsc = emptyFSContext
		return
	}

	// Open the root directory by using "." as "/" is not relevant in fs.FS.
	// This not only validates the file system, but also allows us to test if
	// this is a real file or not. ex. `file.(*os.File)`.
	var rootDir fs.File
	if rdFS, ok := root.(fs.ReadDirFS); ok {
		if rootDir, err = rdFS.Open("."); err != nil {
			return
		}
	} else { // we can't list the root directory, fake it.
		rootDir = emptyRootDir{}
	}

	return &FSContext{
		fs: root,
		openedFiles: map[uint32]*FileEntry{
			FdRoot: {Path: "/", File: rootDir},
		},
		lastFD: FdRoot,
	}, nil
}

// nextFD gets the next file descriptor number in a goroutine safe way (monotonically) or zero if we ran out.
// TODO: openedFiles is still not goroutine safe!
// TODO: This can return zero if we ran out of file descriptors. A future change can optimize by re-using an FD pool.
func (c *FSContext) nextFD() uint32 {
	if c.lastFD == math.MaxUint32 {
		return 0
	}
	return atomic.AddUint32(&c.lastFD, 1)
}

// OpenedFile returns a file and true if it was opened or nil and false, if syscall.EBADF.
func (c *FSContext) OpenedFile(_ context.Context, fd uint32) (*FileEntry, bool) {
	f, ok := c.openedFiles[fd]
	return f, ok
}

// OpenFile is like syscall.Open and returns the file descriptor of the new file or an error.
//
// TODO: Consider dirflags and oflags. Also, allow non-read-only open based on config about the mount.
// e.g. allow os.O_RDONLY, os.O_WRONLY, or os.O_RDWR either by config flag or pattern on filename
// See #390
func (c *FSContext) OpenFile(_ context.Context, name string /* TODO: flags int, perm int */) (uint32, error) {
	f, err := c.openFile(name)
	if err != nil {
		return 0, err
	}

	newFD := c.nextFD()
	if newFD == 0 { // TODO: out of file descriptors
		_ = f.Close()
		return 0, syscall.EBADF
	}
	c.openedFiles[newFD] = &FileEntry{Path: name, File: f}
	return newFD, nil
}

func (c *FSContext) StatFile(_ context.Context, name string) (fs.FileInfo, error) {
	f, err := c.openFile(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (c *FSContext) openFile(name string) (fs.File, error) {
	// fs.ValidFile cannot be rooted (start with '/')
	fsOpenPath := name
	if name[0] == '/' {
		fsOpenPath = name[1:]
	}
	fsOpenPath = path.Clean(fsOpenPath) // e.g. "sub/." -> "sub"

	return c.fs.Open(fsOpenPath)
}

// CloseFile returns true if a file was opened and closed without error, or false if syscall.EBADF.
func (c *FSContext) CloseFile(_ context.Context, fd uint32) bool {
	f, ok := c.openedFiles[fd]
	if !ok {
		return false
	}
	delete(c.openedFiles, fd)

	if err := f.File.Close(); err != nil {
		return false
	}
	return true
}

// Close implements io.Closer
func (c *FSContext) Close(context.Context) (err error) {
	// Close any files opened in this context
	for fd, entry := range c.openedFiles {
		delete(c.openedFiles, fd)
		if e := entry.File.Close(); e != nil {
			err = e // This means err returned == the last non-nil error.
		}
	}
	return
}

// FdWriter returns a valid writer for the given file descriptor or nil if syscall.EBADF.
func FdWriter(ctx context.Context, sysCtx *Context, fd uint32) io.Writer {
	switch fd {
	case FdStdout:
		return sysCtx.Stdout()
	case FdStderr:
		return sysCtx.Stderr()
	case FdRoot:
		return nil // directory, not a writeable file.
	default:
		// Check to see if the file descriptor is available
		if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
			return nil
		} else if writer, ok := f.File.(io.Writer); !ok {
			// Go's syscall.Write also returns EBADF if the FD is present, but not writeable
			return nil
		} else {
			return writer
		}
	}
}

// FdReader returns a valid reader for the given file descriptor or nil if syscall.EBADF.
func FdReader(ctx context.Context, sysCtx *Context, fd uint32) io.Reader {
	if fd == FdStdin {
		return sysCtx.Stdin()
	} else if fd == FdRoot {
		return nil // directory, not a readable file.
	} else if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
		return nil
	} else {
		return f.File
	}
}
