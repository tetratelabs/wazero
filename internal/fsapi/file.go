package fsapi

import (
	"io/fs"
	"syscall"
	"time"
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
	// Ino returns the inode (Stat_t.Ino) of this file, zero if unknown or an
	// error there was an error retrieving it.
	//
	// # Errors
	//
	// Possible errors are those from Stat, except syscall.ENOSYS should not
	// be returned. Zero should be returned if there is no implementation.
	//
	// # Notes
	//
	//   - Some implementations implement this with a cached call to Stat.
	Ino() (uint64, syscall.Errno)

	// IsNonblock returns true if the file was opened with O_NONBLOCK, or
	// SetNonblock was successfully enabled on this file.
	//
	// # Notes
	//
	//   - This might not match the underlying state of the file descriptor if
	//     the file was not opened via OpenFile.
	IsNonblock() bool

	// SetNonblock toggles the non-blocking mode (O_NONBLOCK) of this file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.SetNonblock and `fcntl` with O_NONBLOCK in
	//     POSIX. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/fcntl.html
	SetNonblock(enable bool) syscall.Errno

	// IsAppend returns true if the file was opened with syscall.O_APPEND, or
	// SetAppend was successfully enabled on this file.
	//
	// # Notes
	//
	//   - This might not match the underlying state of the file descriptor if
	//     the file was not opened via OpenFile.
	IsAppend() bool

	// SetAppend toggles the append mode (syscall.O_APPEND) of this file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - There is no `O_APPEND` for `fcntl` in POSIX, so implementations may
	//     have to re-open the underlying file to apply this. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html
	SetAppend(enable bool) syscall.Errno

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

	// IsDir returns true if this file is a directory or an error there was an
	// error retrieving this information.
	//
	// # Errors
	//
	// Possible errors are those from Stat.
	//
	// # Notes
	//
	//   - Some implementations implement this with a cached call to Stat.
	IsDir() (bool, syscall.Errno)

	// Read attempts to read all bytes in the file into `buf`, and returns the
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
	Read(buf []byte) (n int, errno syscall.Errno)

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
	Pread(buf []byte, off int64) (n int, errno syscall.Errno)

	// Seek attempts to set the next offset for Read or Write and returns the
	// resulting absolute offset or an error.
	//
	// # Parameters
	//
	// The `offset` parameters is interpreted in terms of `whence`:
	//   - io.SeekStart: relative to the start of the file, e.g. offset=0 sets
	//     the next Read or Write to the beginning of the file.
	//   - io.SeekCurrent: relative to the current offset, e.g. offset=16 sets
	//     the next Read or Write 16 bytes past the prior.
	//   - io.SeekEnd: relative to the end of the file, e.g. offset=-1 sets the
	//     next Read or Write to the last byte in the file.
	//
	// # Behavior when a directory
	//
	// The only supported use case for a directory is seeking to `offset` zero
	// (`whence` = io.SeekStart). This should have the same behavior as
	// os.File, which resets any internal state used by Readdir.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not readable.
	//   - syscall.EINVAL: the offset was negative.
	//
	// # Notes
	//
	//   - This is like io.Seeker and `fseek` in POSIX, preferring semantics
	//     of io.Seeker. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/fseek.html
	Seek(offset int64, whence int) (newOffset int64, errno syscall.Errno)

	// PollRead returns if the file has data ready to be read or an error.
	//
	// # Parameters
	//
	// The `timeout` parameter when nil blocks up to forever.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//
	// # Notes
	//
	//   - This is like `poll` in POSIX, for a single file.
	//     See https://pubs.opengroup.org/onlinepubs/9699919799/functions/poll.html
	//   - No-op files, such as those which read from /dev/null, should return
	//     immediately true to avoid hangs (because data will never become
	//     available).
	PollRead(timeout *time.Duration) (ready bool, errno syscall.Errno)

	// Readdir reads the contents of the directory associated with file and
	// returns a slice of up to n Dirent values in an arbitrary order. This is
	// a stateful function, so subsequent calls return any next values.
	//
	// If n > 0, Readdir returns at most n entries or an error.
	// If n <= 0, Readdir returns all remaining entries or an error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.ENOTDIR: the file was not a directory
	//
	// # Notes
	//
	//   - This is like `Readdir` on os.File, but unlike `readdir` in POSIX.
	//     See https://pubs.opengroup.org/onlinepubs/9699919799/functions/readdir.html
	//   - For portability reasons, no error is returned at the end of the
	//     directory, when the file is closed or removed while open.
	//     See https://github.com/ziglang/zig/blob/0.10.1/lib/std/fs.zig#L635-L637
	Readdir() (Readdir, syscall.Errno)

	// Write attempts to write all bytes in `p` to the file, and returns the
	// count written even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file was closed, not writeable, or a directory.
	//
	// # Notes
	//
	//   - This is like io.Writer and `write` in POSIX, preferring semantics of
	//     io.Writer. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/write.html
	Write(buf []byte) (n int, errno syscall.Errno)

	// Pwrite attempts to write all bytes in `p` to the file at the given
	// offset `off`, and returns the count written even on error.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed or not writeable.
	//   - syscall.EINVAL: the offset was negative.
	//   - syscall.EISDIR: the file was a directory.
	//
	// # Notes
	//
	//   - This is like io.WriterAt and `pwrite` in POSIX, preferring semantics
	//     of io.WriterAt. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/pwrite.html
	Pwrite(buf []byte, off int64) (n int, errno syscall.Errno)

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

	// Utimens set file access and modification times of this file, at
	// nanosecond precision.
	//
	// # Parameters
	//
	// The `times` parameter includes the access and modification timestamps to
	// assign. Special syscall.Timespec NSec values UTIME_NOW and UTIME_OMIT may be
	// specified instead of real timestamps. A nil `times` parameter behaves the
	// same as if both were set to UTIME_NOW.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.UtimesNano and `futimens` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/futimens.html
	//   - Windows requires files to be open with syscall.O_RDWR, which means you
	//     cannot use this to update timestamps on a directory (syscall.EPERM).
	Utimens(times *[2]syscall.Timespec) syscall.Errno

	// Close closes the underlying file.
	//
	// A zero syscall.Errno is returned if unimplemented or success.
	//
	// # Notes
	//
	//   - This is like syscall.Close and `close` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/close.html
	Close() syscall.Errno
}

// Readdir is the result of File.Readdir and it is a cursor over a directory.
//
// # Errors
//
// All methods that can return an error return a syscall.Errno, which is zero
// on success.
//
// # Notes
//
//   - Implementations are allowed to keep a buffer or a sliding window of values
//     over a directory. Because reading in batches requires to hold a reference
//     to an open directory, callers should pay extra care to make sure
//     that such references are released, by invoking Close.
//   - This is especially important on the Windows platform, where keeping
//     a file handle open may prevent concurrent processes from accessing
//     (e.g. deleting) the underlying directory. This may conflict with cleanup
//     procedures (such as `testing/TB.Cleanup()`).
type Readdir interface {
	// Offset returns the offset of the current value under the internal cursor.
	Offset() uint64

	// Rewind seeks the internal cursor to the given offset.
	//
	// Implementations may keep a buffer or a moving window. The size of
	// the window is an implementation detail, the implementation
	// may return an error if the offset is too small to seek back
	// to that index (i.e. it lays outside the window).
	//
	// Offset 0 is a special case: it is always possible to Rewind to 0.
	// In this case, the Readdir instance is reset to its initial state,
	// discarding any previous buffers.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EINVAL: the offset is larger than the current internal cursor.
	//   - syscall.ENOSYS: the offset is too small for the current window.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is conceptually similar to invoking Seek(offset, io.SeekStart)
	//     on io.Seeker and `fseek` in POSIX over an open directory. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/fseek.html
	//   - However implementations will generally keep a buffer of the currently
	//     valid values and will not actually seek to the given offset the underlying
	//     open directory.
	//   - For offset 0, this is conceptually similar to invoking Seek(0, io.SeekStart)
	//     on io.Seeker and `fseek`, however implementations will generally
	//     reopen the underlying directory for portability reasons.
	Rewind(offset uint64) syscall.Errno

	// Next returns the value currently pointed by the internal cursor,
	// and it advances to the next value.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOENT: there are no more entries to fetch
	//   - syscall.EBADF: the directory is no longer valid
	//   - other error values would signal an issue with fetching the next batch of values.
	//
	// # Notes
	//
	//   - Ultimately, implementations may fetch data using a `readdir`-like API, thus
	//     errors will reflect file system errors similarly to the POSIX call. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/readdir.html
	Next() (*Dirent, syscall.Errno)

	// Close closes the underlying file.
	//
	// # Errors
	//
	// A zero syscall.Errno is returned if unimplemented or success. The below
	// are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.Close and `close` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/close.html
	Close() syscall.Errno
}
