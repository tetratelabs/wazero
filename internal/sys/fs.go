package sys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/experimental/writefs"
	"github.com/tetratelabs/wazero/internal/platform"
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

const (
	modeDevice     = uint32(fs.ModeDevice | 0o640)
	modeCharDevice = uint32(fs.ModeCharDevice | 0o640)
)

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

type stdioFileWriter struct {
	w io.Writer
	s fs.FileInfo
}

// Stat implements fs.File
func (w *stdioFileWriter) Stat() (fs.FileInfo, error) { return w.s, nil }

// Read implements fs.File
func (w *stdioFileWriter) Read([]byte) (n int, err error) {
	return // emulate os.Stdout which returns zero
}

// Write implements io.Writer
func (w *stdioFileWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

// Close implements fs.File
func (w *stdioFileWriter) Close() error {
	// Don't actually close the underlying file, as we didn't open it!
	return nil
}

type stdioFileReader struct {
	r io.Reader
	s fs.FileInfo
}

// Stat implements fs.File
func (r *stdioFileReader) Stat() (fs.FileInfo, error) { return r.s, nil }

// Read implements fs.File
func (r *stdioFileReader) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

// Close implements fs.File
func (r *stdioFileReader) Close() error {
	// Don't actually close the underlying file, as we didn't open it!
	return nil
}

var (
	noopStdinStat  = stdioFileInfo{FdStdin, modeDevice}
	noopStdoutStat = stdioFileInfo{FdStdout, modeDevice}
	noopStderrStat = stdioFileInfo{FdStderr, modeDevice}
)

// stdioFileInfo implements fs.FileInfo where index zero is the FD and one is the mode.
type stdioFileInfo [2]uint32

func (s stdioFileInfo) Name() string {
	switch s[0] {
	case FdStdin:
		return "stdin"
	case FdStdout:
		return "stdout"
	case FdStderr:
		return "stderr"
	default:
		panic(fmt.Errorf("BUG: incorrect FD %d", s[0]))
	}
}

func (stdioFileInfo) Size() int64         { return 0 }
func (s stdioFileInfo) Mode() fs.FileMode { return fs.FileMode(s[1]) }
func (stdioFileInfo) ModTime() time.Time  { return time.Unix(0, 0) }
func (stdioFileInfo) IsDir() bool         { return false }
func (stdioFileInfo) Sys() interface{}    { return nil }

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	// Name is the basename of the file, at the time it was opened. When the
	// file is root "/" (fd = FdRoot), this is "/".
	//
	// Note: This must match fs.FileInfo.
	Name string

	// File is always non-nil, even when root "/" (fd = FdRoot).
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

var errNotDir = errors.New("not a directory")

// NewFSContext creates a FSContext, using the `root` parameter for any paths
// beginning at "/". If the input is EmptyFS, there is no root filesystem.
// Otherwise, `root` is assigned file descriptor FdRoot and the returned
// context can open files in that file system. Any error on opening "." is
// returned.
func NewFSContext(stdin io.Reader, stdout, stderr io.Writer, root fs.FS) (fsc *FSContext, err error) {
	fsc = &FSContext{
		fs: root,
		openedFiles: map[uint32]*FileEntry{
			FdStdin:  stdinReader(stdin),
			FdStdout: stdioWriter(stdout, noopStdoutStat),
			FdStderr: stdioWriter(stderr, noopStderrStat),
		},
		lastFD: FdStderr,
	}

	if root == EmptyFS {
		return fsc, nil
	}

	// Open the root directory by using "." as "/" is not relevant in fs.FS.
	// This not only validates the file system, but also allows us to test if
	// this is a real file or not. ex. `file.(*os.File)`.
	//
	// Note: We don't use fs.ReadDirFS as this isn't implemented by os.DirFS.
	rootDir, err := root.Open(".")
	if err != nil {
		// This could fail because someone made a special-purpose file system,
		// which only passes certain filenames and not ".".
		rootDir = emptyRootDir{}
		err = nil
	}

	// Verify the directory existed and was a directory at the time the context
	// was created.
	var stat fs.FileInfo
	if stat, err = rootDir.Stat(); err != nil {
		return // err if we couldn't determine if the root was a directory.
	} else if !stat.IsDir() {
		err = &fs.PathError{Op: "ReadDir", Path: stat.Name(), Err: errNotDir}
		return
	}

	fsc.openedFiles[FdRoot] = &FileEntry{Name: "/", File: rootDir}
	fsc.lastFD = FdRoot

	return fsc, nil
}

func stdinReader(r io.Reader) *FileEntry {
	if r == nil {
		r = eofReader{}
	}
	s := stdioStat(r, noopStdinStat)
	return &FileEntry{Name: noopStdinStat.Name(), File: &stdioFileReader{r: r, s: s}}
}

func stdioWriter(w io.Writer, defaultStat stdioFileInfo) *FileEntry {
	if w == nil {
		w = io.Discard
	}
	s := stdioStat(w, defaultStat)
	return &FileEntry{Name: defaultStat.Name(), File: &stdioFileWriter{w: w, s: s}}
}

func stdioStat(f interface{}, defaultStat stdioFileInfo) fs.FileInfo {
	if f, ok := f.(*os.File); ok && platform.IsTerminal(f.Fd()) {
		return stdioFileInfo{defaultStat[0], modeCharDevice}
	}
	return defaultStat
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
func (c *FSContext) OpenedFile(fd uint32) (*FileEntry, bool) {
	f, ok := c.openedFiles[fd]
	return f, ok
}

func (c *FSContext) StatFile(fd uint32) (fs.FileInfo, error) {
	f, ok := c.openedFiles[fd]
	if !ok {
		return nil, syscall.EBADF
	}
	return f.File.Stat()
}

// fileModeStat is a fake fs.FileInfo which only returns its mode.
// This is used for character devices.
type fileModeStat fs.FileMode

var _ fs.FileInfo = fileModeStat(0)

func (s fileModeStat) Size() int64        { return 0 }
func (s fileModeStat) Mode() fs.FileMode  { return fs.FileMode(s) }
func (s fileModeStat) ModTime() time.Time { return time.Unix(0, 0) }
func (s fileModeStat) Sys() interface{}   { return nil }
func (s fileModeStat) Name() string       { return "" }
func (s fileModeStat) IsDir() bool        { return false }

// Mkdir is like syscall.Mkdir and returns the file descriptor of the new
// directory or an error.
func (c *FSContext) Mkdir(name string, perm fs.FileMode) (newFD uint32, err error) {
	name = c.cleanPath(name)
	if wfs, ok := c.fs.(writefs.FS); ok {
		if err = wfs.Mkdir(name, perm); err != nil {
			return
		}
		// TODO: Determine how to handle when a directory already
		// exists or is a file.
		return c.OpenFile(name, os.O_RDONLY, perm)
	}
	err = syscall.ENOSYS
	return
}

// Unlink is like syscall.Unlink.
func (c *FSContext) Unlink(name string) (err error) {
	if wfs, ok := c.fs.(writefs.FS); ok {
		name = c.cleanPath(name)
		return wfs.Remove(name)
	}
	err = syscall.ENOSYS
	return
}

// OpenFile is like syscall.Open and returns the file descriptor of the new file or an error.
func (c *FSContext) OpenFile(name string, flags int, perm fs.FileMode) (newFD uint32, err error) {
	create := flags&os.O_CREATE != 0
	var f fs.File
	if wfs, ok := c.fs.(writefs.FS); ok {
		name = c.cleanPath(name)
		f, err = wfs.OpenFile(name, flags, perm)
	} else if create {
		err = syscall.ENOSYS
	} else {
		f, err = c.openFile(name)
	}

	if err != nil {
		return 0, err
	}

	newFD = c.nextFD()
	if newFD == 0 { // TODO: out of file descriptors
		_ = f.Close()
		return 0, syscall.EBADF
	}
	c.openedFiles[newFD] = &FileEntry{Name: path.Base(name), File: f}
	return newFD, nil
}

func (c *FSContext) StatPath(name string) (fs.FileInfo, error) {
	f, err := c.openFile(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (c *FSContext) openFile(name string) (fs.File, error) {
	return c.fs.Open(c.cleanPath(name))
}

func (c *FSContext) cleanPath(name string) string {
	// fs.ValidFile cannot be rooted (start with '/')
	fsOpenPath := name
	if name[0] == '/' {
		fsOpenPath = name[1:]
	}
	fsOpenPath = path.Clean(fsOpenPath) // e.g. "sub/." -> "sub"
	return fsOpenPath
}

// FdWriter returns a valid writer for the given file descriptor or nil if syscall.EBADF.
func (c *FSContext) FdWriter(fd uint32) io.Writer {
	// Check to see if the file descriptor is available
	if f, ok := c.openedFiles[fd]; !ok {
		return nil
	} else if writer, ok := f.File.(io.Writer); !ok {
		// Go's syscall.Write also returns EBADF if the FD is present, but not writeable
		return nil
	} else {
		return writer
	}
}

// FdReader returns a valid reader for the given file descriptor or nil if syscall.EBADF.
func (c *FSContext) FdReader(fd uint32) io.Reader {
	switch fd {
	case FdStdout, FdStderr:
		return nil // writer, not a readable file.
	case FdRoot:
		return nil // directory, not a readable file.
	}

	if f, ok := c.openedFiles[fd]; !ok {
		return nil // TODO: could be a directory not a file.
	} else {
		return f.File
	}
}

// CloseFile returns true if a file was opened and closed without error, or false if syscall.EBADF.
func (c *FSContext) CloseFile(fd uint32) bool {
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

// Close implements api.Closer
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
