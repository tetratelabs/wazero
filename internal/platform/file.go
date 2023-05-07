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
	// Path returns path used to open the file or empty if not applicable. For
	// example, a file representing stdout will return empty.
	//
	// Note: This can drift on rename.
	Path() string

	// AccessMode returns the access mode the file was opened with.
	//
	// This returns exclusively one of the following:
	//   - syscall.O_RDONLY: read-only, e.g. os.Stdin
	//   - syscall.O_WRONLY: write-only, e.g. os.Stdout
	//   - syscall.O_RDWR: read-write, e.g. os.CreateTemp
	AccessMode() int

	// Stat is similar to syscall.Fstat.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Fstat and `fstatat` with `AT_FDCWD` in POSIX.
	//     See https://pubs.opengroup.org/onlinepubs/9699919799/functions/stat.html
	//   - A fs.FileInfo backed implementation sets atim, mtim and ctim to the
	//     same value.
	//   - Windows allows you to stat a closed directory.
	Stat() (Stat_t, syscall.Errno)

	// Read attempts to read all bytes in the file into `p`, and returns the
	// count read even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not readable.
	//   - syscall.EISDIR: the file was a directory.
	//
	// # Notes
	//
	//   - This is like io.Reader and `read` in POSIX, preferring semantics of
	//     io.Reader. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/read.html
	//   - Unlike io.Reader, there is no io.EOF returned on end-of-file. To
	//     read the file completely, the caller must repeat until `n` is zero.
	Read(p []byte) (n int, errno syscall.Errno)

	// Pread attempts to read all bytes in the file into `p`, starting at the
	// offset `off`, and returns the count read even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not readable.
	//   - syscall.EINVAL: the offset was negative.
	//   - syscall.EISDIR: the file was a directory.
	//
	// # Notes
	//
	//   - This is like io.ReaderAt and `pread` in POSIX, preferring semantics
	//     of io.ReaderAt. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/pread.html
	//   - Unlike io.ReaderAt, there is no io.EOF returned on end-of-file. To
	//     read the file completely, the caller must repeat until `n` is zero.
	Pread(p []byte, off int64) (n int, errno syscall.Errno)

	// Write attempts to write all bytes in `p` to the file, and returns the
	// count written even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not writeable.
	//
	// # Notes
	//
	//   - This is like io.Writer and `write` in POSIX, preferring semantics of
	//     io.Writer. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/write.html
	Write(p []byte) (n int, errno syscall.Errno)

	// Pwrite attempts to write all bytes in `p` to the file at the given
	// offset `off`, and returns the count written even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not writeable.
	//   - syscall.EINVAL: the offset was negative.
	//
	// # Notes
	//
	//   - This is like io.WriterAt and `pwrite` in POSIX, preferring semantics
	//     of io.WriterAt. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/pwrite.html
	Pwrite(p []byte, off int64) (n int, errno syscall.Errno)

	// Truncate truncates a file to a specified length.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//   - syscall.EINVAL: the `size` is negative.
	//   - syscall.EISDIR: the file was a directory.
	//
	// # Notes
	//
	//   - This is like syscall.Ftruncate and `ftruncate` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/ftruncate.html
	//   - Windows does not error when calling Truncate on a closed file.
	Truncate(size int64) syscall.Errno

	// Sync synchronizes changes to the file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Fsync and `fsync` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fsync.html
	//   - This returns with no error instead of syscall.ENOSYS when
	//     unimplemented. This prevents fake filesystems from erring.
	//   - Windows does not error when calling Sync on a closed file.
	Sync() syscall.Errno

	// Datasync synchronizes the data of a file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Fdatasync and `fdatasync` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fdatasync.html
	//   - This returns with no error instead of syscall.ENOSYS when
	//     unimplemented. This prevents fake filesystems from erring.
	//   - As this is commonly missing, some implementations dispatch to Sync.
	Datasync() syscall.Errno

	// Chmod changes the mode of the file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Fchmod and `fchmod` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fchmod.html
	//   - Windows ignores the execute bit, and any permissions come back as
	//     group and world. For example, chmod of 0400 reads back as 0444, and
	//     0700 0666. Also, permissions on directories aren't supported at all.
	Chmod(fs.FileMode) syscall.Errno

	// Chown changes the owner and group of a file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Fchown and `fchown` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fchown.html
	//   - This always returns syscall.ENOSYS on windows.
	Chown(uid, gid int) syscall.Errno

	// Close closes the underlying file.
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//
	// # Notes
	//
	//   - This is like syscall.Close and `close` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/close.html
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

// Read implements File.Read
func (UnimplementedFile) Read([]byte) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Pread implements File.Pread
func (UnimplementedFile) Pread([]byte, int64) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Write implements File.Write
func (UnimplementedFile) Write([]byte) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Pwrite implements File.Pwrite
func (UnimplementedFile) Pwrite([]byte, int64) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Truncate implements File.Truncate
func (UnimplementedFile) Truncate(int64) syscall.Errno {
	return syscall.ENOSYS
}

// Sync implements File.Sync
func (UnimplementedFile) Sync() syscall.Errno {
	return 0 // not syscall.ENOSYS
}

// Datasync implements File.Datasync
func (UnimplementedFile) Datasync() syscall.Errno {
	return 0 // not syscall.ENOSYS
}

// Chmod implements File.Chmod
func (UnimplementedFile) Chmod(fs.FileMode) syscall.Errno {
	return syscall.ENOSYS
}

// Chown implements File.Chown
func (UnimplementedFile) Chown(int, int) syscall.Errno {
	return syscall.ENOSYS
}

func NewFsFile(openPath string, openFlag int, f fs.File) File {
	return &fsFile{
		path:       openPath,
		accessMode: openFlag & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR),
		file:       f,
	}
}

type fsFile struct {
	path       string
	accessMode int
	file       fs.File
}

// Path implements File.Path
func (f *fsFile) Path() string {
	return f.path
}

// AccessMode implements File.AccessMode
func (f *fsFile) AccessMode() int {
	return f.accessMode
}

// Stat implements File.Stat
func (f *fsFile) Stat() (Stat_t, syscall.Errno) {
	st, errno := statFile(f.file)
	if errno == syscall.EIO {
		errno = syscall.EBADF
	}
	return st, errno
}

// Read implements File.Read
func (f *fsFile) Read(p []byte) (n int, errno syscall.Errno) {
	if len(p) == 0 {
		return 0, 0 // less overhead on zero-length reads.
	}

	if f.accessMode == syscall.O_WRONLY {
		return 0, syscall.EBADF
	}
	if w, ok := f.File().(io.Reader); ok {
		n, err := w.Read(p)
		return n, UnwrapOSError(err)
	}
	return 0, syscall.EBADF
}

// Pread implements File.Pread
func (f *fsFile) Pread(p []byte, off int64) (n int, errno syscall.Errno) {
	if len(p) == 0 {
		return 0, 0 // less overhead on zero-length reads.
	}

	if f.accessMode == syscall.O_WRONLY {
		return 0, syscall.EBADF
	}

	// Simple case, handle with io.ReaderAt.
	if w, ok := f.File().(io.ReaderAt); ok {
		n, err := w.ReadAt(p, off)
		return n, UnwrapOSError(err)
	}

	// See /RATIONALE.md "fd_pread: io.Seeker fallback when io.ReaderAt is not supported"
	if rs, ok := f.File().(io.ReadSeeker); ok {
		// Determine the current position in the file, as we need to revert it.
		currentOffset, err := rs.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, UnwrapOSError(err)
		}

		// Put the read position back when complete.
		defer func() { _, _ = rs.Seek(currentOffset, io.SeekStart) }()

		// If the current offset isn't in sync with this reader, move it.
		if off != currentOffset {
			if _, err = rs.Seek(off, io.SeekStart); err != nil {
				return 0, UnwrapOSError(err)
			}
		}

		n, err := rs.Read(p)
		return n, UnwrapOSError(err)
	}

	return 0, syscall.ENOSYS // unsupported
}

// Write implements File.Write
func (f *fsFile) Write(p []byte) (n int, errno syscall.Errno) {
	if len(p) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	if f.accessMode == syscall.O_RDONLY {
		return 0, syscall.EBADF
	}
	if w, ok := f.File().(io.Writer); ok {
		n, err := w.Write(p)
		return n, UnwrapOSError(err)
	}
	return 0, syscall.ENOSYS // unsupported
}

// Pwrite implements File.Pwrite
func (f *fsFile) Pwrite(p []byte, off int64) (n int, errno syscall.Errno) {
	if len(p) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	if f.accessMode == syscall.O_RDONLY {
		return 0, syscall.EBADF
	}
	if w, ok := f.File().(io.WriterAt); ok {
		n, err := w.WriteAt(p, off)
		return n, UnwrapOSError(err)
	}
	return 0, syscall.ENOSYS // unsupported
}

// Truncate implements File.Truncate
func (f *fsFile) Truncate(size int64) syscall.Errno {
	if tf, ok := f.file.(truncateFile); ok {
		errno := UnwrapOSError(tf.Truncate(size))
		if errno == 0 {
			return 0
		}

		// Operating systems return different syscall.Errno instead of EISDIR
		// double-check on any err until we can assure this per OS.
		if isOpenDir(f) {
			return syscall.EISDIR
		}
		return errno
	}
	return syscall.ENOSYS
}

// Sync implements File.Sync
func (f *fsFile) Sync() syscall.Errno {
	return sync(f.file)
}

// Datasync implements File.Datasync
func (f *fsFile) Datasync() syscall.Errno {
	return datasync(f.file)
}

// Chmod implements File.Chmod
func (f *fsFile) Chmod(mode fs.FileMode) syscall.Errno {
	if f, ok := f.file.(chmodFile); ok {
		return UnwrapOSError(f.Chmod(mode))
	}
	return syscall.ENOSYS
}

// Chown implements File.Chown
func (f *fsFile) Chown(uid, gid int) syscall.Errno {
	if f, ok := f.file.(fdFile); ok {
		return fchown(f.Fd(), uid, gid)
	}
	return syscall.ENOSYS
}

// Close implements File.Close
func (f *fsFile) Close() syscall.Errno {
	return UnwrapOSError(f.file.Close())
}

// File implements File.File
func (f *fsFile) File() fs.File {
	return f.file
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

func isOpenDir(f File) bool {
	if st, statErrno := f.Stat(); statErrno == 0 && st.Mode.IsDir() {
		return true
	}
	return false
}
