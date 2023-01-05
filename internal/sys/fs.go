package sys

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/syscallfs"
)

const (
	FdStdin uint32 = iota
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

const (
	modeDevice     = uint32(fs.ModeDevice | 0o640)
	modeCharDevice = uint32(fs.ModeCharDevice | 0o640)
)

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

type lazyDir struct {
	fs syscallfs.FS
	f  fs.File
}

// Stat implements fs.File
func (r *lazyDir) Stat() (fs.FileInfo, error) {
	if f, err := r.file(); err != nil {
		return nil, err
	} else {
		return f.Stat()
	}
}

func (r *lazyDir) file() (f fs.File, err error) {
	if f = r.f; r.f != nil {
		return
	}
	r.f, err = r.fs.OpenFile(".", os.O_RDONLY, 0)
	f = r.f
	return
}

// Read implements fs.File
func (r *lazyDir) Read(p []byte) (n int, err error) {
	if f, err := r.file(); err != nil {
		return 0, err
	} else {
		return f.Read(p)
	}
}

// Close implements fs.File
func (r *lazyDir) Close() error {
	if f, err := r.file(); err != nil {
		return nil
	} else {
		return f.Close()
	}
}

// FileEntry maps a path to an open file in a file system.
type FileEntry struct {
	// Name is the name of the directory up to its pre-open.
	//
	// Note: This is empty when a pre-open and can drift on rename.
	Name string

	// IsPreopen is a directory that is lazily opened.
	IsPreopen bool

	isDirectory bool

	// File is always non-nil.
	File fs.File

	// ReadDir is present when this File is a fs.ReadDirFile and `ReadDir`
	// was called.
	ReadDir *ReadDir
}

// IsDir returns true if the file is a directory.
func (f *FileEntry) IsDir() bool {
	if f.IsPreopen || f.isDirectory {
		return true
	}
	_, _ = f.Stat() // Maybe the file hasn't had stat yet.
	return f.isDirectory
}

// Stat returns the underlying stat of this file.
func (f *FileEntry) Stat() (stat fs.FileInfo, err error) {
	stat, err = f.File.Stat()
	if err == nil && stat.IsDir() {
		f.isDirectory = true
	}
	return stat, err
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

	// openedFiles is a map of file descriptor numbers (>=FdPreopen) to open files
	// (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles FileTable
}

// NewFSContext creates a FSContext with stdio streams and an optional
// pre-opened filesystem.
//
// If `preopened` is not syscallfs.EmptyFS, it is inserted into the file
// descriptor table as FdPreopen.
func NewFSContext(stdin io.Reader, stdout, stderr io.Writer, preopened syscallfs.FS) (fsc *FSContext, err error) {
	fsc = &FSContext{fs: preopened}
	fsc.openedFiles.Insert(stdinReader(stdin))
	fsc.openedFiles.Insert(stdioWriter(stdout, noopStdoutStat))
	fsc.openedFiles.Insert(stdioWriter(stderr, noopStderrStat))

	if preopened == syscallfs.EmptyFS {
		return fsc, nil
	}

	fsc.openedFiles.Insert(&FileEntry{
		IsPreopen: true,
		File:      &lazyDir{fs: preopened},
	})
	return fsc, nil
}

func stdinReader(r io.Reader) *FileEntry {
	if r == nil {
		r = eofReader{}
	}
	s := stdioStat(r, noopStdinStat)
	return &FileEntry{File: &stdioFileReader{r: r, s: s}}
}

func stdioWriter(w io.Writer, defaultStat stdioFileInfo) *FileEntry {
	if w == nil {
		w = io.Discard
	}
	s := stdioStat(w, defaultStat)
	return &FileEntry{File: &stdioFileWriter{w: w, s: s}}
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
		if path == "/" || path == "." {
			path = ""
		}
		newFD := c.openedFiles.Insert(&FileEntry{Name: path, File: f})
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

// WriterForFile returns a writer for the given file descriptor or nil if not
// opened or not writeable (e.g. a directory or a file not opened for writes).
func WriterForFile(fsc *FSContext, fd uint32) (writer io.Writer) {
	if f, ok := fsc.LookupFile(fd); ok {
		writer = f.File.(io.Writer)
	}
	return
}
