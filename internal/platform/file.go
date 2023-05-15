package platform

import (
	"io"
	"io/fs"
	"os"
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

	// AccessMode returns the access mode the file was opened with.
	//
	// This returns exclusively one of the following:
	//   - syscall.O_RDONLY: read-only, e.g. os.Stdin
	//   - syscall.O_WRONLY: write-only, e.g. os.Stdout
	//   - syscall.O_RDWR: read-write, e.g. os.CreateTemp
	AccessMode() int
	// ^-- TODO: see if we can remove this

	// IsNonblock returns true if SetNonblock was successfully enabled on this
	// file.
	//
	// # Notes
	//
	//   - This may not match the underlying state of the file descriptor if it
	//     was opened (OpenFile) in non-blocking mode.
	IsNonblock() bool
	// ^-- TODO: We should be able to cache the open flag and remove this note.

	// SetNonblock toggles the non-blocking mode of this file.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//   - syscall.EBADF: the file or directory was closed.
	//
	// # Notes
	//
	//   - This is like syscall.SetNonblock and `fcntl` with `O_NONBLOCK` in
	//     POSIX. See https://pubs.opengroup.org/onlinepubs/9699919799/functions/fcntl.html
	SetNonblock(enable bool) syscall.Errno

	// IsAppend returns true if SetAppend was successfully enabled on this file.
	//
	// # Notes
	//
	//   - This might not match the underlying state of the file descriptor if
	//     it was opened (OpenFile) in append mode.
	IsAppend() bool
	// ^-- TODO: We should be able to cache the open flag and remove this note.

	// SetAppend toggles the append mode of this file.
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
	Readdir(n int) (dirents []Dirent, errno syscall.Errno)
	// ^-- TODO: consider being more like POSIX, for example, returning a
	// closeable Dirent object that can iterate on demand. This would
	// centralize sizing logic needed by wasi, particularly extra dirents
	// stored in the sys.FileEntry type. It could possibly reduce the need to
	// reopen the whole file.

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
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOSYS: the implementation does not support this function.
	//
	// # Notes
	//
	//   - This is like syscall.Close and `close` in POSIX. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/close.html
	Close() syscall.Errno
}

// UnimplementedFile is a File that returns syscall.ENOSYS for all functions,
// This should be embedded to have forward compatible implementations.
type UnimplementedFile struct{}

// Ino implements File.Ino
func (UnimplementedFile) Ino() (uint64, syscall.Errno) {
	return 0, 0
}

// IsAppend implements File.IsAppend
func (UnimplementedFile) IsAppend() bool {
	return false
}

// SetAppend implements File.SetAppend
func (UnimplementedFile) SetAppend(bool) syscall.Errno {
	return syscall.ENOSYS
}

// IsNonblock implements File.IsNonblock
func (UnimplementedFile) IsNonblock() bool {
	return false
}

// SetNonblock implements File.SetNonblock
func (UnimplementedFile) SetNonblock(bool) syscall.Errno {
	return syscall.ENOSYS
}

// Stat implements File.Stat
func (UnimplementedFile) Stat() (Stat_t, syscall.Errno) {
	return Stat_t{}, syscall.ENOSYS
}

// IsDir implements File.IsDir
func (UnimplementedFile) IsDir() (bool, syscall.Errno) {
	return false, syscall.ENOSYS
}

// Read implements File.Read
func (UnimplementedFile) Read([]byte) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Pread implements File.Pread
func (UnimplementedFile) Pread([]byte, int64) (int, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Seek implements File.Seek
func (UnimplementedFile) Seek(int64, int) (int64, syscall.Errno) {
	return 0, syscall.ENOSYS
}

// Readdir implements File.Readdir
func (UnimplementedFile) Readdir(int) (dirents []Dirent, errno syscall.Errno) {
	return nil, syscall.ENOSYS
}

// PollRead implements File.PollRead
func (UnimplementedFile) PollRead(*time.Duration) (ready bool, errno syscall.Errno) {
	return false, syscall.ENOSYS
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

// Utimens implements File.Utimens
func (UnimplementedFile) Utimens(*[2]syscall.Timespec) syscall.Errno {
	return syscall.ENOSYS
}

func NewStdioFile(stdin bool, f fs.File) (File, error) {
	// Return constant stat, which has fake times, but keep the underlying
	// file mode. Fake times are needed to pass wasi-testsuite.
	// https://github.com/WebAssembly/wasi-testsuite/blob/af57727/tests/rust/src/bin/fd_filestat_get.rs#L1-L19
	var mode fs.FileMode
	if st, err := f.Stat(); err != nil {
		return nil, err
	} else {
		mode = st.Mode()
	}
	var accessMode int
	if stdin {
		accessMode = syscall.O_RDONLY
	} else {
		accessMode = syscall.O_WRONLY
	}
	var file File
	if of, ok := f.(*os.File); ok {
		// This is ok because functions that need path aren't used by stdioFile
		file = newOsFile("", accessMode, 0, of)
	} else {
		file = &fsFile{accessMode: accessMode, file: f}
	}
	return &stdioFile{File: file, st: Stat_t{Mode: mode, Nlink: 1}}, nil
}

func OpenFile(path string, flag int, perm fs.FileMode) (*os.File, syscall.Errno) {
	if flag&O_DIRECTORY != 0 && flag&(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, syscall.EISDIR // invalid to open a directory writeable
	}
	return openFile(path, flag, perm)
}

func OpenOSFile(path string, flag int, perm fs.FileMode) (File, syscall.Errno) {
	f, errno := OpenFile(path, flag, perm)
	if errno != 0 {
		return nil, errno
	}
	return newOsFile(path, flag, perm, f), 0
}

func OpenFSFile(fs fs.FS, path string, flag int, perm fs.FileMode) (File, syscall.Errno) {
	if flag&O_DIRECTORY != 0 && flag&(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, syscall.EISDIR // invalid to open a directory writeable
	}
	f, err := fs.Open(path)
	if errno := UnwrapOSError(err); errno != 0 {
		return nil, errno
	}
	// Don't return an os.File because the path is not absolute. osFile needs
	// the path to be real and certain fs.File impls are subrooted.
	return &fsFile{
		fs:         fs,
		name:       path,
		accessMode: flag & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR),
		file:       f,
	}, 0
}

type stdioFile struct {
	File
	st Stat_t
}

// IsDir implements File.IsDir
func (f *stdioFile) IsDir() (bool, syscall.Errno) {
	return false, 0
}

// Stat implements File.Stat
func (f *stdioFile) Stat() (Stat_t, syscall.Errno) {
	return f.st, 0
}

// Close implements File.Close
func (f *stdioFile) Close() syscall.Errno {
	return 0
}

// fsFile is used for wrapped os.File, like os.Stdin or any fs.File
// implementation. Notably, this does not have access to the full file path.
// so certain operations can't be supported, such as inode lookups on Windows.
type fsFile struct {
	UnimplementedFile

	// fs is the file-system that opened the file, or nil when wrapped for
	// pre-opens like stdio.
	fs fs.FS

	// name is what was used in fs for Open, so it may not be the actual path.
	name string

	// accessMode is only set when OpenFSFile opened this file.
	accessMode int

	// file is always set, possibly an os.File like os.Stdin.
	file fs.File

	// closed is true when closed was called. This ensures proper syscall.EBADF
	closed bool

	// cachedStat includes fields that won't change while a file is open.
	cachedSt *cachedStat
}

type cachedStat struct {
	// fileType is the same as what's documented on Dirent.
	fileType fs.FileMode

	// ino is the same as what's documented on Dirent.
	ino uint64
}

// cachedStat returns the cacheable parts of platform.Stat_t or an error if
// they couldn't be retrieved.
func (f *fsFile) cachedStat() (fileType fs.FileMode, ino uint64, errno syscall.Errno) {
	if f.cachedSt == nil {
		if _, errno = f.Stat(); errno != 0 {
			return
		}
	}
	return f.cachedSt.fileType, f.cachedSt.ino, 0
}

// Ino implements File.Ino
func (f *fsFile) Ino() (uint64, syscall.Errno) {
	if _, ino, errno := f.cachedStat(); errno != 0 {
		return 0, errno
	} else {
		return ino, 0
	}
}

// AccessMode implements File.AccessMode
func (f *fsFile) AccessMode() int {
	return f.accessMode
}

// IsAppend implements File.IsAppend
func (f *fsFile) IsAppend() bool {
	return false
}

// SetAppend implements File.SetAppend
func (f *fsFile) SetAppend(bool) (errno syscall.Errno) {
	return fileError(f, f.closed, syscall.ENOSYS)
}

// IsDir implements File.IsDir
func (f *fsFile) IsDir() (bool, syscall.Errno) {
	if ft, _, errno := f.cachedStat(); errno != 0 {
		return false, errno
	} else if ft.Type() == fs.ModeDir {
		return true, 0
	}
	return false, 0
}

// Stat implements File.Stat
func (f *fsFile) Stat() (st Stat_t, errno syscall.Errno) {
	if f.closed {
		errno = syscall.EBADF
		return
	}

	// While some functions in platform.File need the full path, especially in
	// Windows, stat does not. Casting here allows os.DirFS to return inode
	// information.
	if of, ok := f.file.(*os.File); ok {
		if st, errno = statFile(of); errno != 0 {
			return
		}
		return f.cacheStat(st)
	} else if t, err := f.file.Stat(); err != nil {
		errno = UnwrapOSError(err)
		return
	} else {
		st = statFromDefaultFileInfo(t)
		return f.cacheStat(st)
	}
}

func (f *fsFile) cacheStat(st Stat_t) (Stat_t, syscall.Errno) {
	f.cachedSt = &cachedStat{fileType: st.Mode & fs.ModeType, ino: st.Ino}
	return st, 0
}

// Read implements File.Read
func (f *fsFile) Read(buf []byte) (n int, errno syscall.Errno) {
	if n, errno = read(f.file, buf); errno != 0 {
		// Defer validation overhead until we've already had an error.
		errno = fileError(f, f.closed, errno)
	}
	return
}

// Pread implements File.Pread
func (f *fsFile) Pread(buf []byte, off int64) (n int, errno syscall.Errno) {
	if ra, ok := f.file.(io.ReaderAt); ok {
		if n, errno = pread(ra, buf, off); errno != 0 {
			// Defer validation overhead until we've already had an error.
			errno = fileError(f, f.closed, errno)
		}
		return
	}

	// See /RATIONALE.md "fd_pread: io.Seeker fallback when io.ReaderAt is not supported"
	if rs, ok := f.file.(io.ReadSeeker); ok {
		// Determine the current position in the file, as we need to revert it.
		currentOffset, err := rs.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, fileError(f, f.closed, UnwrapOSError(err))
		}

		// Put the read position back when complete.
		defer func() { _, _ = rs.Seek(currentOffset, io.SeekStart) }()

		// If the current offset isn't in sync with this reader, move it.
		if off != currentOffset {
			if _, err = rs.Seek(off, io.SeekStart); err != nil {
				return 0, fileError(f, f.closed, UnwrapOSError(err))
			}
		}

		n, err = rs.Read(buf)
		if errno = UnwrapOSError(err); errno != 0 {
			// Defer validation overhead until we've already had an error.
			errno = fileError(f, f.closed, errno)
		}
	} else {
		errno = syscall.ENOSYS // unsupported
	}
	return
}

// Seek implements File.Seek.
func (f *fsFile) Seek(offset int64, whence int) (newOffset int64, errno syscall.Errno) {
	// If this is a directory, and we're attempting to seek to position zero,
	// we have to re-open the file to ensure the directory state is reset.
	var isDir bool
	if offset == 0 && whence == io.SeekStart {
		if isDir, errno = f.IsDir(); errno != 0 {
			return
		} else if isDir {
			return 0, f.reopen()
		}
	}

	if s, ok := f.file.(io.Seeker); ok {
		if newOffset, errno = seek(s, offset, whence); errno != 0 {
			// Defer validation overhead until we've already had an error.
			errno = fileError(f, f.closed, errno)
		}
	} else {
		errno = syscall.ENOSYS // unsupported
	}
	return
}

func (f *fsFile) reopen() syscall.Errno {
	_ = f.close()
	var err error
	f.file, err = f.fs.Open(f.name)
	return UnwrapOSError(err)
}

// Readdir implements File.Readdir. Notably, this uses fs.ReadDirFile if
// available.
func (f *fsFile) Readdir(n int) (dirents []Dirent, errno syscall.Errno) {
	if of, ok := f.file.(*os.File); ok {
		// We can't use f.name here because it is the path up to the fs.FS, not
		// necessarily the real path. For this reason, Windows may not be able
		// to populate inodes. However, Darwin and Linux will.
		if dirents, errno = readdir(of, "", n); errno != 0 {
			errno = adjustReaddirErr(f, f.closed, errno)
		}
		return
	}

	// Try with fs.ReadDirFile which is available on fs.FS implementations
	// like embed:fs.
	if rdf, ok := f.file.(fs.ReadDirFile); ok {
		entries, e := rdf.ReadDir(n)
		if errno = adjustReaddirErr(f, f.closed, e); errno != 0 {
			return
		}
		dirents = make([]Dirent, 0, len(entries))
		for _, e := range entries {
			// By default, we don't attempt to read inode data
			dirents = append(dirents, Dirent{Name: e.Name(), Type: e.Type()})
		}
	} else {
		errno = syscall.ENOTDIR
	}
	return
}

// Write implements File.Write
func (f *fsFile) Write(buf []byte) (n int, errno syscall.Errno) {
	if w, ok := f.file.(io.Writer); ok {
		if n, errno = write(w, buf); errno != 0 {
			// Defer validation overhead until we've already had an error.
			errno = fileError(f, f.closed, errno)
		}
	} else {
		errno = syscall.ENOSYS // unsupported
	}
	return
}

// Pwrite implements File.Pwrite
func (f *fsFile) Pwrite(buf []byte, off int64) (n int, errno syscall.Errno) {
	if wa, ok := f.file.(io.WriterAt); ok {
		if n, errno = pwrite(wa, buf, off); errno != 0 {
			// Defer validation overhead until we've already had an error.
			errno = fileError(f, f.closed, errno)
		}
	} else {
		errno = syscall.ENOSYS // unsupported
	}
	return
}

// Close implements File.Close
func (f *fsFile) Close() syscall.Errno {
	if f.closed {
		return 0
	}
	f.closed = true
	return f.close()
}

func (f *fsFile) close() syscall.Errno {
	return UnwrapOSError(f.file.Close())
}

// dirError is used for commands that work against a directory, but not a file.
func dirError(f File, isClosed bool, errno syscall.Errno) syscall.Errno {
	if vErrno := validate(f, isClosed, false, true); vErrno != 0 {
		return vErrno
	}
	return errno
}

// fileError is used for commands that work against a file, but not a directory.
func fileError(f File, isClosed bool, errno syscall.Errno) syscall.Errno {
	if vErrno := validate(f, isClosed, true, false); vErrno != 0 {
		return vErrno
	}
	return errno
}

// validate is used to making syscalls which will fail.
func validate(f File, isClosed, wantFile, wantDir bool) syscall.Errno {
	if isClosed {
		return syscall.EBADF
	}

	isDir, errno := f.IsDir()
	if errno != 0 {
		return errno
	}

	if wantFile && isDir {
		return syscall.EISDIR
	} else if wantDir && !isDir {
		return syscall.ENOTDIR
	}
	return 0
}

func read(r io.Reader, buf []byte) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length reads.
	}

	n, err := r.Read(buf)
	return n, UnwrapOSError(err)
}

func pread(ra io.ReaderAt, buf []byte, off int64) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length reads.
	}

	n, err := ra.ReadAt(buf, off)
	return n, UnwrapOSError(err)
}

func seek(s io.Seeker, offset int64, whence int) (int64, syscall.Errno) {
	if uint(whence) > io.SeekEnd {
		return 0, syscall.EINVAL // negative or exceeds the largest valid whence
	}

	newOffset, err := s.Seek(offset, whence)
	return newOffset, UnwrapOSError(err)
}

func readdir(f *os.File, path string, n int) (dirents []Dirent, errno syscall.Errno) {
	fis, e := f.Readdir(n)
	if errno = UnwrapOSError(e); errno != 0 {
		return
	}

	dirents = make([]Dirent, 0, len(fis))

	// linux/darwin won't have to fan out to lstat, but windows will.
	var ino uint64
	for fi := range fis {
		t := fis[fi]
		if ino, errno = inoFromFileInfo(path, t); errno != 0 {
			return
		}
		dirents = append(dirents, Dirent{Name: t.Name(), Ino: ino, Type: t.Mode().Type()})
	}
	return
}

func write(w io.Writer, buf []byte) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	n, err := w.Write(buf)
	return n, UnwrapOSError(err)
}

func pwrite(w io.WriterAt, buf []byte, off int64) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	n, err := w.WriteAt(buf, off)
	return n, UnwrapOSError(err)
}
