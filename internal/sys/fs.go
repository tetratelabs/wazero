package sys

import (
	"context"
	"io"
	"io/fs"
	"math"
	"sync/atomic"
	"syscall"
)

const (
	FdStdin = iota
	FdStdout
	FdStderr
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

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	Path string
	// File when nil this is the root "/" (fd=3)
	File fs.File
}

type FSContext struct {
	// fs is the root ("/") mount.
	fs fs.FS

	// openedFiles is a map of file descriptor numbers (>=3) to open files (or directories) and defaults to empty.
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

// NewFSContext returns a mutable context if the fs is not EmptyFS.
func NewFSContext(fs fs.FS) *FSContext {
	if fs == EmptyFS {
		return emptyFSContext
	}
	return &FSContext{
		fs: fs,
		openedFiles: map[uint32]*FileEntry{
			3: {Path: "/"}, // after STDERR
		},
		lastFD: 3,
	}
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
// Ex. allow os.O_RDONLY, os.O_WRONLY, or os.O_RDWR either by config flag or pattern on filename
// See #390
func (c *FSContext) OpenFile(_ context.Context, name string /* TODO: flags int, perm int */) (uint32, error) {
	// fs.ValidFile cannot start with '/'
	fsOpenPath := name
	if name[0] == '/' {
		fsOpenPath = name[1:]
	}

	f, err := c.fs.Open(fsOpenPath)
	if err != nil {
		return 0, err // Don't wrap the underlying error which is already a PathError!
	}

	newFD := c.nextFD()
	if newFD == 0 {
		_ = f.Close()
		return 0, syscall.EBADF
	}
	c.openedFiles[newFD] = &FileEntry{Path: name, File: f}
	return newFD, nil
}

// CloseFile returns true if a file was opened and closed without error, or false if syscall.EBADF.
func (c *FSContext) CloseFile(_ context.Context, fd uint32) bool {
	f, ok := c.openedFiles[fd]
	if !ok {
		return false
	}
	delete(c.openedFiles, fd)

	if f.File == nil { // The root entry
		return true
	}
	if err := f.File.Close(); err != nil {
		return false
	}
	return true
}

// Close implements io.Closer
func (c *FSContext) Close(_ context.Context) (err error) {
	// Close any files opened in this context
	for fd, entry := range c.openedFiles {
		delete(c.openedFiles, fd)
		if entry.File != nil { // File is nil for the root filesystem
			if e := entry.File.Close(); e != nil {
				err = e // This means the err returned == the last non-nil error.
			}
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
	default:
		// Check to see if the file descriptor is available
		if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok || f.File == nil {
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
	} else if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
		return nil
	} else {
		return f.File
	}
}
