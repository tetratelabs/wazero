package platform

import (
	"io"
	"io/fs"
	"syscall"
)

// File is a writeable fs.File bridge backed by syscall functions needed for ABI
// including WASI and runtime.GOOS=js.
//
// Implementations should embed UnimplementedFile for forward compatability. Any
// unsupported method or parameter should return syscall.ENOSYS.
//
// # Errors
//
// All methods that can return an error return a syscall.Errno, which is zero
// on success.
//
// Restricting to syscall.Errno matches current WebAssembly host functions,
// which are constrained to well-known error codes. For example, `GOOS=js` maps
// hard coded values and panics otherwise. More commonly, WASI maps syscall
// errors to u32 numeric values.
//
// # Notes
//
// A writable filesystem abstraction is not yet implemented as of Go 1.20. See
// https://github.com/golang/go/issues/45757
type File interface {
	// Stat is similar to syscall.Fstat.
	//
	// # Errors
	//
	// The following errors are expected:
	//   - syscall.EBADF if the file or directory was closed.
	//
	// # Notes
	//
	//   - An fs.FileInfo backed implementation sets atim, mtim and ctim to the
	//     same value.
	//   - Windows allows you to stat a closed directory.
	Stat() (Stat_t, syscall.Errno)

	// Close closes the underlying file.
	Close() syscall.Errno

	// File is temporary until we port other methods.
	File() fs.File
}

// UnimplementedFile is a File that returns syscall.ENOSYS for all functions,
// This should be embedded to have forward compatible implementations.
type UnimplementedFile struct{}

// Stat implements File.Stat
func (UnimplementedFile) Stat() (Stat_t, syscall.Errno) {
	return Stat_t{}, syscall.ENOSYS
}

type DefaultFile struct {
	F fs.File
}

// Stat implements File.Stat
func (f *DefaultFile) Stat() (Stat_t, syscall.Errno) {
	st, errno := statFile(f.F)
	if errno == syscall.EIO {
		errno = syscall.EBADF
	}
	return st, errno
}

// Close implements File.Close
func (f *DefaultFile) Close() syscall.Errno {
	return UnwrapOSError(f.F.Close())
}

// File implements File.File
func (f *DefaultFile) File() fs.File {
	return f.F
}

// ReadFile declares all read interfaces defined on os.File used by wazero.
type ReadFile interface {
	fdFile // for the number of links.
	readdirnamesFile
	readdirFile
	fs.ReadDirFile
	io.ReaderAt // for pread
	io.Seeker   // fallback for ReaderAt for embed:fs
}

// WriteFile declares all interfaces defined on os.File used by wazero.
type WriteFile interface {
	ReadFile
	io.Writer
	io.WriterAt // for pwrite
	chmodFile
	syncFile
	truncateFile
}

// The following interfaces are used until we finalize our own FD-scoped file.
type (
	// PathFile is implemented on files that retain the path to their pre-open.
	PathFile interface {
		Path() string
	}
	// fdFile is implemented by os.File in file_unix.go and file_windows.go
	fdFile interface{ Fd() (fd uintptr) }
	// readdirnamesFile is implemented by os.File in dir.go
	readdirnamesFile interface {
		Readdirnames(n int) (names []string, err error)
	}
	// readdirFile is implemented by os.File in dir.go
	readdirFile interface {
		Readdir(n int) ([]fs.FileInfo, error)
	}
	// chmodFile is implemented by os.File in file_posix.go
	chmodFile interface{ Chmod(fs.FileMode) error }
	// syncFile is implemented by os.File in file_posix.go
	syncFile interface{ Sync() error }
	// truncateFile is implemented by os.File in file_posix.go
	truncateFile interface{ Truncate(size int64) error }
)
