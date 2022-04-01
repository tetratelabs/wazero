package wasm

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync/atomic"
)

// FileEntry maps a path to an open file in a file system.
//
// Note: This does not introduce cycles because the types here are in the package "wasi" not "internalwasi".
type FileEntry struct {
	Path string
	FS   fs.FS
	// File when nil this is a mount like "." or "/".
	File fs.File
}

// SysContext holds module-scoped system resources currently only used by internalwasi.
type SysContext struct {
	args, environ         []string
	argsSize, environSize uint32
	stdin                 io.Reader
	stdout, stderr        io.Writer

	// openedFiles is a map of file descriptor numbers (>=3) to open files (or directories) and defaults to empty.
	// TODO: This is unguarded, so not goroutine-safe!
	openedFiles map[uint32]*FileEntry

	// lastFD is not meant to be read directly. Rather by nextFD.
	lastFD uint32
}

// nextFD gets the next file descriptor number in a goroutine safe way (monotonically) or zero if we ran out.
// TODO: opendFiles is still not goroutine safe!
// TODO: This can return zero if we ran out of file descriptors. A future change can optimize by re-using an FD pool.
func (c *SysContext) nextFD() uint32 {
	if c.lastFD == math.MaxUint32 {
		return 0
	}
	return atomic.AddUint32(&c.lastFD, 1)
}

// Args is like os.Args and defaults to nil.
//
// Note: The count will never be more than math.MaxUint32.
// See wazero.SysConfig WithArgs
func (c *SysContext) Args() []string {
	return c.args
}

// ArgsSize is the size to encode Args as Null-terminated strings.
//
// Note: To get the size without null-terminators, subtract the length of Args from this value.
// See wazero.SysConfig WithArgs
// See https://en.wikipedia.org/wiki/Null-terminated_string
func (c *SysContext) ArgsSize() uint32 {
	return c.argsSize
}

// Environ are "key=value" entries like os.Environ and default to nil.
//
// Note: The count will never be more than math.MaxUint32.
// See wazero.SysConfig WithEnviron
func (c *SysContext) Environ() []string {
	return c.environ
}

// EnvironSize is the size to encode Environ as Null-terminated strings.
//
// Note: To get the size without null-terminators, subtract the length of Environ from this value.
// See wazero.SysConfig WithEnviron
// See https://en.wikipedia.org/wiki/Null-terminated_string
func (c *SysContext) EnvironSize() uint32 {
	return c.environSize
}

// Stdin is like exec.Cmd Stdin and defaults to a reader of os.DevNull.
// See wazero.SysConfig WithStdin
func (c *SysContext) Stdin() io.Reader {
	return c.stdin
}

// Stdout is like exec.Cmd Stdout and defaults to io.Discard.
// See wazero.SysConfig WithStdout
func (c *SysContext) Stdout() io.Writer {
	return c.stdout
}

// Stderr is like exec.Cmd Stderr and defaults to io.Discard.
// See wazero.SysConfig WithStderr
func (c *SysContext) Stderr() io.Writer {
	return c.stderr
}

// eofReader is safer than reading from os.DevNull as it can never overrun operating system file descriptors.
type eofReader struct{}

// Read implements io.Reader
// Note: This doesn't use a pointer reference as it has no state and an empty struct doesn't allocate.
func (eofReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

// DefaultSysContext returns SysContext with no values set.
//
// Note: This isn't a constant because SysContext.openedFiles is currently mutable even when empty.
// TODO: Make it an error to open or close files when no FS was assigned.
func DefaultSysContext() *SysContext {
	if sys, err := NewSysContext(0, nil, nil, nil, nil, nil, nil); err != nil {
		panic(fmt.Errorf("BUG: DefaultSysContext should never error: %w", err))
	} else {
		return sys
	}
}

var _ = DefaultSysContext() // Force panic on bug.

// NewSysContext is a factory function which helps avoid needing to know defaults or exporting all fields.
// Note: max is exposed for testing. max is only used for env/args validation.
func NewSysContext(max uint32, args, environ []string, stdin io.Reader, stdout, stderr io.Writer, openedFiles map[uint32]*FileEntry) (sys *SysContext, err error) {
	sys = &SysContext{args: args, environ: environ}

	if sys.argsSize, err = nullTerminatedByteCount(max, args); err != nil {
		return nil, fmt.Errorf("args invalid: %w", err)
	}

	if sys.environSize, err = nullTerminatedByteCount(max, environ); err != nil {
		return nil, fmt.Errorf("environ invalid: %w", err)
	}

	if stdin == nil {
		sys.stdin = eofReader{}
	} else {
		sys.stdin = stdin
	}

	if stdout == nil {
		sys.stdout = io.Discard
	} else {
		sys.stdout = stdout
	}

	if stderr == nil {
		sys.stderr = io.Discard
	} else {
		sys.stderr = stderr
	}

	if openedFiles == nil {
		sys.openedFiles = map[uint32]*FileEntry{}
		sys.lastFD = 2 // STDERR
	} else {
		sys.openedFiles = openedFiles
		sys.lastFD = 2 // STDERR
		for fd := range openedFiles {
			if fd > sys.lastFD {
				sys.lastFD = fd
			}
		}
	}
	return
}

// nullTerminatedByteCount ensures the count or Nul-terminated length of the elements doesn't exceed max, and that no
// element includes the nul character.
func nullTerminatedByteCount(max uint32, elements []string) (uint32, error) {
	count := uint32(len(elements))
	if count > max {
		return 0, errors.New("exceeds maximum count")
	}

	// The buffer size is the total size including null terminators. The null terminator count == value count, sum
	// count with each value length. This works because in Go, the length of a string is the same as its byte count.
	bufSize, maxSize := uint64(count), uint64(max) // uint64 to allow summing without overflow
	for _, e := range elements {
		// As this is null-terminated, We have to validate there are no null characters in the string.
		for _, c := range e {
			if c == 0 {
				return 0, errors.New("contains NUL character")
			}
		}

		nextSize := bufSize + uint64(len(e))
		if nextSize > maxSize {
			return 0, errors.New("exceeds maximum size")
		}
		bufSize = nextSize

	}
	return uint32(bufSize), nil
}

// Close implements io.Closer
func (c *SysContext) Close() (err error) {
	// Close any files opened in this context
	for fd, entry := range c.openedFiles {
		delete(c.openedFiles, fd)
		if entry.File != nil { // File is nil for a mount like "." or "/"
			if e := entry.File.Close(); e != nil {
				err = e // This means the err returned == the last non-nil error.
			}
		}
	}
	return
}

// CloseFile returns true if a file was opened and closed without error, or false if not.
func (c *SysContext) CloseFile(fd uint32) (bool, error) {
	f, ok := c.openedFiles[fd]
	if !ok {
		return false, nil
	}
	delete(c.openedFiles, fd)

	if f.File == nil { // TODO: currently, this means it is a pre-opened filesystem, but this may change later.
		return true, nil
	}
	if err := f.File.Close(); err != nil {
		return false, err
	}
	return true, nil
}

// OpenedFile returns a file and true if it was opened or nil and false, if not.
func (c *SysContext) OpenedFile(fd uint32) (*FileEntry, bool) {
	f, ok := c.openedFiles[fd]
	return f, ok
}

// OpenFile returns the file descriptor of the new file or false if we ran out of file descriptors
func (c *SysContext) OpenFile(f *FileEntry) (uint32, bool) {
	newFD := c.nextFD()
	if newFD == 0 {
		return 0, false
	}
	c.openedFiles[newFD] = f
	return newFD, true
}
