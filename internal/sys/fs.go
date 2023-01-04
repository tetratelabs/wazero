package sys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/syscallfs"
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
	fs syscallfs.FS

	// openedFiles is a map of file descriptor numbers (>=FdRoot) to open files
	// (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles FileTable
}

var errNotDir = errors.New("not a directory")

// NewFSContext creates a FSContext, using the `root` parameter for any paths
// beginning at "/". If the input is EmptyFS, there is no root filesystem.
// Otherwise, `root` is assigned file descriptor FdRoot and the returned
// context can open files in that file system. Any error on opening "." is
// returned.
func NewFSContext(stdin io.Reader, stdout, stderr io.Writer, root syscallfs.FS) (fsc *FSContext, err error) {
	fsc = &FSContext{fs: root}
	fsc.openedFiles.Insert(stdinReader(stdin))
	fsc.openedFiles.Insert(stdioWriter(stdout, noopStdoutStat))
	fsc.openedFiles.Insert(stdioWriter(stderr, noopStderrStat))

	if root == syscallfs.EmptyFS {
		return fsc, nil
	}

	// Test if this is a real file or not. ex. `file.(*os.File)`.
	rootDir, err := root.OpenFile(".", os.O_RDONLY, 0)
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

	fsc.openedFiles.Insert(&FileEntry{Name: "/", File: rootDir})
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

// FS returns the underlying filesystem. Any files that should be added to the
// table should be inserted via InsertFile.
func (c *FSContext) FS() syscallfs.FS {
	return c.fs
}

// OpenFile opens the file into the table and returns its file descriptor.
// The result must be closed by CloseFile or Close.
func (c *FSContext) OpenFile(path string, flag int, perm fs.FileMode) (uint32, error) {
	if f, err := c.fs.OpenFile(path, flag, perm); err != nil {
		return 0, err
	} else {
		newFD := c.openedFiles.Insert(&FileEntry{Name: pathutil.Base(path), File: f})
		return newFD, nil
	}
}

// LookupFile returns a file if it is in the table.
func (c *FSContext) LookupFile(fd uint32) (*FileEntry, bool) {
	f, ok := c.openedFiles.Lookup(fd)
	return f, ok
}

// CloseFile returns any error closing the existing file.
func (c *FSContext) CloseFile(fd uint32) error {
	f, ok := c.openedFiles.Lookup(fd)
	if !ok {
		return syscall.EBADF
	}
	c.openedFiles.Delete(fd)
	return f.File.Close()
}

// Close implements api.Closer
func (c *FSContext) Close(context.Context) (err error) {
	// Close any files opened in this context
	c.openedFiles.Range(func(fd uint32, entry *FileEntry) bool {
		if e := entry.File.Close(); e != nil {
			err = e // This means err returned == the last non-nil error.
		}
		return true
	})
	// A closed FSContext cannot be reused so clear the state instead of
	// using Reset.
	c.openedFiles = FileTable{}
	return
}

// StatFile is a convenience that calls FSContext.LookupFile then fs.File Stat.
// syscall.EBADF is returned on lookup failure.
func StatFile(fsc *FSContext, fd uint32) (stat fs.FileInfo, err error) {
	if f, ok := fsc.LookupFile(fd); !ok {
		err = syscall.EBADF
	} else {
		stat, err = f.File.Stat()
	}
	return
}

// WriterForFile returns a writer for the given file descriptor or nil if not
// opened or not writeable (e.g. a directory or a file not opened for writes).
func WriterForFile(fsc *FSContext, fd uint32) (writer io.Writer) {
	if f, ok := fsc.LookupFile(fd); ok {
		writer = f.File.(io.Writer)
	}
	return
}
