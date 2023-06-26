package sysfs

import (
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

func NewStdioFile(stdin bool, f fs.File) (fsapi.File, error) {
	// Return constant stat, which has fake times, but keep the underlying
	// file mode. Fake times are needed to pass wasi-testsuite.
	// https://github.com/WebAssembly/wasi-testsuite/blob/af57727/tests/rust/src/bin/fd_filestat_get.rs#L1-L19
	var mode fs.FileMode
	if st, err := f.Stat(); err != nil {
		return nil, err
	} else {
		mode = st.Mode()
	}
	var flag int
	if stdin {
		flag = syscall.O_RDONLY
	} else {
		flag = syscall.O_WRONLY
	}
	var file fsapi.File
	if of, ok := f.(*os.File); ok {
		// This is ok because functions that need path aren't used by stdioFile
		file = newOsFile("", flag, 0, of)
	} else {
		file = &fsFile{file: f}
	}
	return &stdioFile{File: file, st: fsapi.Stat_t{Mode: mode, Nlink: 1}}, nil
}

func OpenFile(path string, flag int, perm fs.FileMode) (*os.File, syscall.Errno) {
	if flag&fsapi.O_DIRECTORY != 0 && flag&(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, syscall.EISDIR // invalid to open a directory writeable
	}
	return openFile(path, flag, perm)
}

func OpenOSFile(path string, flag int, perm fs.FileMode) (fsapi.File, syscall.Errno) {
	f, errno := OpenFile(path, flag, perm)
	if errno != 0 {
		return nil, errno
	}
	return newOsFile(path, flag, perm, f), 0
}

func OpenFSFile(fs fs.FS, path string, flag int, perm fs.FileMode) (fsapi.File, syscall.Errno) {
	if flag&fsapi.O_DIRECTORY != 0 && flag&(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, syscall.EISDIR // invalid to open a directory writeable
	}
	f, err := fs.Open(path)
	if errno := platform.UnwrapOSError(err); errno != 0 {
		return nil, errno
	}
	// Don't return an os.File because the path is not absolute. osFile needs
	// the path to be real and certain fs.File impls are subrooted.
	return &fsFile{fs: fs, name: path, file: f}, 0
}

type stdioFile struct {
	fsapi.File
	st fsapi.Stat_t
}

// SetAppend implements File.SetAppend
func (f *stdioFile) SetAppend(bool) syscall.Errno {
	// Ignore for stdio.
	return 0
}

// IsAppend implements File.SetAppend
func (f *stdioFile) IsAppend() bool {
	return true
}

// IsDir implements File.IsDir
func (f *stdioFile) IsDir() (bool, syscall.Errno) {
	return false, 0
}

// Stat implements File.Stat
func (f *stdioFile) Stat() (fsapi.Stat_t, syscall.Errno) {
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
	fsapi.UnimplementedFile

	// fs is the file-system that opened the file, or nil when wrapped for
	// pre-opens like stdio.
	fs fs.FS

	// name is what was used in fs for Open, so it may not be the actual path.
	name string

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

// cachedStat returns the cacheable parts of platform.sys.Stat_t or an error if
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
func (f *fsFile) Stat() (st fsapi.Stat_t, errno syscall.Errno) {
	if f.closed {
		errno = syscall.EBADF
		return
	}

	// While some functions in fsapi.File need the full path, especially in
	// Windows, stat does not. Casting here allows os.DirFS to return inode
	// information.
	if of, ok := f.file.(*os.File); ok {
		if st, errno = statFile(of); errno != 0 {
			return
		}
		return f.cacheStat(st)
	} else if t, err := f.file.Stat(); err != nil {
		errno = platform.UnwrapOSError(err)
		return
	} else {
		st = StatFromDefaultFileInfo(t)
		return f.cacheStat(st)
	}
}

func (f *fsFile) cacheStat(st fsapi.Stat_t) (fsapi.Stat_t, syscall.Errno) {
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
			return 0, fileError(f, f.closed, platform.UnwrapOSError(err))
		}

		// Put the read position back when complete.
		defer func() { _, _ = rs.Seek(currentOffset, io.SeekStart) }()

		// If the current offset isn't in sync with this reader, move it.
		if off != currentOffset {
			if _, err = rs.Seek(off, io.SeekStart); err != nil {
				return 0, fileError(f, f.closed, platform.UnwrapOSError(err))
			}
		}

		n, err = rs.Read(buf)
		if errno = platform.UnwrapOSError(err); errno != 0 {
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
	// If the file failed to reopen (e.g. deleted in the meantime), then we flip the closed bit.
	if err != nil {
		f.closed = true
		return platform.UnwrapOSError(err)
	}
	return 0
}

// Readdir implements File.Readdir. Notably, this uses fs.ReadDirFile if
// available.
func (f *fsFile) Readdir() (dirs fsapi.Readdir, errno syscall.Errno) {
	if _, ok := f.file.(*os.File); ok {
		// We can't use f.name here because it is the path up to the fsapi.FS,
		// not necessarily the real path. For this reason, Windows may not be
		// able to populate inodes. However, Darwin and Linux will.
		if dirs, errno = newReaddirFromFile(f, ""); errno != 0 {
			errno = adjustReaddirErr(f, f.closed, errno)
		}
		return
	}

	// Try with fs.ReadDirFile which is available on api.FS implementations
	// like embed:fs.
	if rdf, ok := f.file.(fs.ReadDirFile); ok {
		entries, e := rdf.ReadDir(-1)
		if errno = adjustReaddirErr(f, f.closed, e); errno != 0 {
			return
		}
		dirents := make([]fsapi.Dirent, 0, 2+len(entries))
		for _, e := range entries {
			// By default, we don't attempt to read inode data
			dirents = append(dirents, fsapi.Dirent{Name: e.Name(), Type: e.Type()})
		}
		return NewReaddir(dirents...), 0
	} else {
		return emptyReaddir{}, syscall.ENOTDIR
	}
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
	return platform.UnwrapOSError(f.file.Close())
}

// dirError is used for commands that work against a directory, but not a file.
func dirError(f fsapi.File, isClosed bool, errno syscall.Errno) syscall.Errno {
	if vErrno := validate(f, isClosed, false, true); vErrno != 0 {
		return vErrno
	}
	return errno
}

// fileError is used for commands that work against a file, but not a directory.
func fileError(f fsapi.File, isClosed bool, errno syscall.Errno) syscall.Errno {
	if vErrno := validate(f, isClosed, true, false); vErrno != 0 {
		return vErrno
	}
	return errno
}

// validate is used to making syscalls which will fail.
func validate(f fsapi.File, isClosed, wantFile, wantDir bool) syscall.Errno {
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
	return n, platform.UnwrapOSError(err)
}

func pread(ra io.ReaderAt, buf []byte, off int64) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length reads.
	}

	n, err := ra.ReadAt(buf, off)
	return n, platform.UnwrapOSError(err)
}

func seek(s io.Seeker, offset int64, whence int) (int64, syscall.Errno) {
	if uint(whence) > io.SeekEnd {
		return 0, syscall.EINVAL // negative or exceeds the largest valid whence
	}

	newOffset, err := s.Seek(offset, whence)
	return newOffset, platform.UnwrapOSError(err)
}

// rawOsFile exposes the underlying *os.File of an fsapi.File implementation.
//
// It is unexported because it is only used internally by newReaddirFromFile.
// The implementation of an fsapi.File may mutate its own underlying *os.File
// reference: notably, on Windows (esp. Go 1.18) it is not possible to Seek(0)
// a directory. The only way to do it, is closing the directory and update
// the corresponding reference (usually a field called `file`).
//
// Thus, we need to be able to hold a reference to the fsapi.File
// and access only that specific `file` field.
//
// Capturing the underlying `file` field would capture that specific reference;
// thus, if the `file` reference is updated, the captured value would point
// to an old/invalid file descriptor.
type rawOsFile interface {
	fsapi.File

	// rawOsFile returns the underlying *os.File instance to this fsapi.File.
	//
	// # Notes
	//
	//   - Due to how the internal *os.File reference may mutate,
	//     you should only reference it through this method, and never
	//     capture it, or assign it to a field of a different struct,
	//     unless you are sure that the lifetime of that captured reference
	//     will not outlive the lifetime of this reference.
	rawOsFile() *os.File

	// dup duplicates this rawOsFile instance.
	//
	// Implementations may choose different strategies, but generally
	// the safest way to duplicate the handle is to reopen it.
	// Thus, the errors will report inconsistent states of the file system
	// such as when a file was deleted while trying to reopen it.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EINVAL: the file was not valid.
	//   - syscall.ENOENT: the file or directory did not exist.
	//
	// # Notes
	//
	//   - This is conceptually similar to and `dup` in POSIX, hence the name. See
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/dup.html
	//   - However, this being generally implemented in terms of `open`, see also
	//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html
	dup() (rawOsFile, syscall.Errno)
}

// compile-time check to ensure *fsFile implements rawOsFile.
var _ rawOsFile = (*fsFile)(nil)

// rawOsFile implements the same method as documented on rawOsFile.
func (f *fsFile) rawOsFile() *os.File {
	return f.file.(*os.File)
}

// dup implements the same method as documented on rawOsFile.
func (f *fsFile) dup() (rawOsFile, syscall.Errno) {
	file, err := f.fs.Open(f.name)
	if err != nil {
		if file != nil {
			file.Close()
		}
		// fs.Open returns ErrInvalid (EINVAL) or ErrNotExist (ENOENT).
		return nil, platform.UnwrapOSError(err)
	}

	return &fsFile{
		fs:       f.fs,
		name:     f.name,
		file:     file,
		closed:   false,
		cachedSt: f.cachedSt,
	}, 0
}

func write(w io.Writer, buf []byte) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	n, err := w.Write(buf)
	return n, platform.UnwrapOSError(err)
}

func pwrite(w io.WriterAt, buf []byte, off int64) (n int, errno syscall.Errno) {
	if len(buf) == 0 {
		return 0, 0 // less overhead on zero-length writes.
	}

	n, err := w.WriteAt(buf, off)
	return n, platform.UnwrapOSError(err)
}

// compile-time check to ensure windowedReaddir implements fsapi.Readdir.
var _ fsapi.Readdir = (*emptyReaddir)(nil)

// emptyReaddir implements fsapi.Readdir
//
// emptyReaddir is an empty fsapi.Readdir.
type emptyReaddir struct{}

// Offset implements the same method as documented on fsapi.Readdir.
func (e emptyReaddir) Offset() uint64 { return 0 }

// Rewind implements the same method as documented on fsapi.Readdir.
func (e emptyReaddir) Rewind(offset uint64) syscall.Errno { return 0 }

// Next implements the same method as documented on fsapi.Readdir.
func (e emptyReaddir) Next() (*fsapi.Dirent, syscall.Errno) { return nil, syscall.ENOENT }

// Close implements the same method as documented on fsapi.Readdir.
func (emptyReaddir) Close() syscall.Errno { return 0 }

// compile-time check to ensure sliceReaddir implements fsapi.Readdir.
var _ fsapi.Readdir = (*sliceReaddir)(nil)

// sliceReaddir implements fsapi.Readdir
//
// sliceReaddir is a cursor over externally defined dirents.
type sliceReaddir struct {
	// cursor is the current position in the buffer.
	cursor  uint64
	dirents []fsapi.Dirent
}

// NewReaddir creates an instance from externally defined directory entries.
func NewReaddir(dirents ...fsapi.Dirent) fsapi.Readdir {
	return &sliceReaddir{dirents: dirents}
}

// Offset implements the same method as documented on fsapi.Readdir.
func (s *sliceReaddir) Offset() uint64 {
	return s.cursor
}

// Rewind implements the same method as documented on fsapi.Readdir.
func (s *sliceReaddir) Rewind(offset uint64) syscall.Errno {
	switch {
	case offset > s.cursor:
		// The offset cannot be larger than the cursor.
		return syscall.EINVAL
	case offset == 0 && s.cursor == 0:
		return 0
	case offset == 0 && s.cursor != 0:
		// This means that there was a previous call to the dir, but offset is reset.
		// This happens when the program calls rewinddir, for example:
		// https://github.com/WebAssembly/wasi-libc/blob/659ff414560721b1660a19685110e484a081c3d4/libc-bottom-half/cloudlibc/src/libc/dirent/rewinddir.c#L10-L12
		s.cursor = 0
		return 0
	case offset < s.cursor:
		// We are allowed to rewind back to a previous offset within the current window.
		s.cursor = offset
		return 0
	default:
		// The offset is valid.
		return 0
	}
}

// Next implements the same method as documented on fsapi.Readdir.
func (s *sliceReaddir) Next() (*fsapi.Dirent, syscall.Errno) {
	if s.cursor >= uint64(len(s.dirents)) {
		return nil, syscall.ENOENT
	}
	d := &s.dirents[s.cursor]
	s.cursor++
	return d, 0
}

// Close implements the same method as documented on fsapi.Readdir.
func (e *sliceReaddir) Close() syscall.Errno { return 0 }

// compile-time check to ensure concatReaddir implements fsapi.Readdir.
var _ fsapi.Readdir = (*concatReaddir)(nil)

// concatReaddir implements fsapi.Readdir
//
// concatReaddir concatenates two fsapi.Readdir instances.
type concatReaddir struct {
	first, second, current fsapi.Readdir
}

// NewConcatReaddir is a constructor for an fsapi.Readdir that concatenates
// two fsapi.Readdir.
func NewConcatReaddir(first fsapi.Readdir, second fsapi.Readdir) fsapi.Readdir {
	return &concatReaddir{first: first, second: second, current: first}
}

// Offset implements the same method as documented on fsapi.Readdir.
func (c *concatReaddir) Offset() uint64 {
	return c.first.Offset() + c.second.Offset()
}

// Rewind implements the same method as documented on fsapi.Readdir.
func (c *concatReaddir) Rewind(offset uint64) syscall.Errno {
	if offset > c.first.Offset() {
		return c.second.Rewind(offset - c.first.Offset())
	} else {
		c.current = c.first
		if errno := c.second.Rewind(0); errno != 0 {
			return errno
		}
		return c.first.Rewind(offset)
	}
}

// Next implements the same method as documented on fsapi.Readdir.
func (c *concatReaddir) Next() (*fsapi.Dirent, syscall.Errno) {
	if d, errno := c.current.Next(); errno == syscall.ENOENT {
		if c.current != c.second {
			c.current = c.second
			d, errno = c.current.Next()
		}
		return d, errno
	} else if errno != 0 {
		return nil, errno
	} else {
		return d, 0
	}
}

// Close implements the same method as documented on fsapi.Readdir.
func (c *concatReaddir) Close() syscall.Errno {
	err1 := c.first.Close()
	err2 := c.second.Close()
	// Return at least one of the error codes.
	if err1 != 0 {
		return err1
	}
	if err2 != 0 {
		return err2
	}
	return 0
}

const direntBufSize = 16

// compile-time check to ensure windowedReaddir implements fsapi.Readdir.
var _ fsapi.Readdir = (*windowedReaddir)(nil)

// windowedReaddir implements fsapi.Readdir
//
// windowedReaddir iterates over the contents of a directory,
// lazily fetching data to a moving buffer window.
type windowedReaddir struct {
	// cursor is the total count of files read including Dirents.
	//
	// Notes:
	//
	// * cursor is the index of the next file in the list. This is
	//   also the value that Cookie returns, so it should always be
	//   higher or equal than the cookie given in Rewind.
	//
	// * this can overflow to negative, which means our implementation
	//   doesn't support writing greater than max int64 entries.
	//   cursor uint64
	cursor uint64

	// window is an fsapi.Readdir over a fixed buffer of size direntBufSize.
	// Notably, directory listing are not rewindable, so we keep entries around
	// in case the caller mis-estimated their buffer and needs a few still cached.
	window fsapi.Readdir

	// init is called on startup and on Rewind(0).
	//
	// It may be used to reset an internal cursor, seek a directory
	// to its beginning, closing and reopening a file etc.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EINVAL: the file was not valid.
	//   - syscall.ENOENT: the file or directory did not exist.
	init func() syscall.Errno

	// fetch fetches a new batch of direntBufSize elements.
	//
	// It may be used to reset an internal cursor, seek a directory
	// to its beginning, closing and reopening a file etc.
	//
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.ENOENT: there are no more entries to fetch
	//   - other error values would signal an issue with fetching the next batch of values.
	fetch func(n uint64) (fsapi.Readdir, syscall.Errno)

	// close closes the underliying implementation.
	// # Errors
	//
	// A zero syscall.Errno is success. The below are expected otherwise:
	//   - syscall.EINVAL: the file was not valid.
	//   - syscall.EBADF: the file was already closed.
	close func() syscall.Errno
}

// newWindowedReaddir is a constructor for Readdir. It takes a dirInit
func newWindowedReaddir(
	init func() syscall.Errno,
	fetch func(n uint64) (fsapi.Readdir, syscall.Errno),
	close func() syscall.Errno,
) (fsapi.Readdir, syscall.Errno) {
	d := &windowedReaddir{init: init, fetch: fetch, close: close, window: emptyReaddir{}}
	errno := d.reset()
	if errno != 0 {
		d.Close()
		return emptyReaddir{}, errno
	} else {
		return d, 0
	}
}

// reset zeroes the cursor and invokes the fetch method to reset
// the internal state of the Readdir struct.
func (d *windowedReaddir) reset() syscall.Errno {
	errno := d.init()
	if errno != 0 {
		return errno
	}
	d.cursor = 0
	dir, errno := d.fetch(uint64(direntBufSize))
	if errno != 0 {
		return errno
	}
	d.window = dir
	return 0
}

// Offset implements the same method as documented on fsapi.Readdir.
//
// Note: this returns the cursor field, but it is an implementation detail.
func (d *windowedReaddir) Offset() uint64 {
	return d.cursor
}

// Rewind implements the same method as documented on fsapi.Readdir.
func (d *windowedReaddir) Rewind(offset uint64) syscall.Errno {
	switch {
	case offset > d.cursor:
		// The offset cannot be larger than cursor.
		return syscall.EINVAL
	case offset == 0:
		// This means that there was a previous call to the dir, but cookie is reset.
		// This happens when the program calls rewinddir, for example:
		// https://github.com/WebAssembly/wasi-libc/blob/659ff414560721b1660a19685110e484a081c3d4/libc-bottom-half/cloudlibc/src/libc/dirent/rewinddir.c#L10-L12
		return d.reset()
	case offset < d.cursor:
		if offset/direntBufSize != d.cursor/direntBufSize {
			// The cookie is not 0, but it points into a window before the current one.
			// If the offset is exactly one element before the current cursor.
			if offset == d.cursor-1 && d.cursor%direntBufSize == 0 {
				d.cursor = offset
				return d.window.Rewind(offset % direntBufSize)
			}
			return syscall.ENOSYS
		}
		// We are allowed to rewind back to a previous offset within the current window.
		d.cursor = offset
		// d.cursor = d.cursor % direntBufSize
		return d.window.Rewind(d.cursor % direntBufSize)
	default:
		// The cookie is valid.
		return 0
	}
}

// Next implements the same method as documented on fsapi.Readdir.
//
// This implementation empties and refill the buffer with the next
// set of values when the internal cursor reaches the end of it.
func (d *windowedReaddir) Next() (*fsapi.Dirent, syscall.Errno) {
	if dirent, errno := d.window.Next(); errno == syscall.ENOENT {
		if window, errno := d.fetch(direntBufSize); errno != 0 {
			return nil, errno
		} else if window == nil {
			return nil, syscall.ENOENT
		} else {
			d.cursor++
			d.window = window
			return d.window.Next()
		}
	} else if errno != 0 {
		return nil, errno
	} else {
		d.cursor++
		return dirent, 0
	}
}

// Close implements the same method as documented on fsapi.Readdir.
func (d *windowedReaddir) Close() syscall.Errno {
	return d.close()
}

// newReaddirFromFile captures a reference to the given rawOsFile (fsapi.File subtype)
// and it fetches the directory listing to an underlying windowedReaddir.
//
// It is important that the fetch function captures a reference to an fsapi.File
// rather than a *os.File, otherwise we may be mistakenly capturing a reference
// that could be invalidated: *os.File references may mutate during the lifetime of
// an fsapi.File.
//
// See also docs for rawOsFile.
func newReaddirFromFile(f rawOsFile, path string) (fsapi.Readdir, syscall.Errno) {
	var file rawOsFile
	init := func() (errno syscall.Errno) {
		if file != nil {
			file.Close()
		}
		// Reopen the directory from path to make sure that
		// we seek to the start correctly on all platforms.
		file, errno = f.dup()
		if errno != 0 && file != nil {
			file.Close()
		}
		return
	}

	fetch := func(n uint64) (fsapi.Readdir, syscall.Errno) {
		fis, err := file.rawOsFile().Readdir(int(n))
		if err == io.EOF {
			return emptyReaddir{}, 0
		}
		if errno := platform.UnwrapOSError(err); errno != 0 {
			return nil, errno
		}
		dirents := make([]fsapi.Dirent, 0, len(fis))

		// linux/darwin won't have to fan out to lstat, but windows will.
		// var ino uint64
		for fi := range fis {
			t := fis[fi]
			if ino, errno := inoFromFileInfo(path, t); errno != 0 {
				return nil, errno
			} else {
				dirents = append(dirents, fsapi.Dirent{Name: t.Name(), Ino: ino, Type: t.Mode().Type()})
			}
		}
		return NewReaddir(dirents...), 0
	}

	close := func() syscall.Errno {
		if file != nil {
			return platform.UnwrapOSError(file.Close())
		} else {
			return 0
		}
	}

	return newWindowedReaddir(init, fetch, close)
}

// ReaddirAll reads eagerly all the values returned by the given
// Readdir instance and returns a slice or a syscall.Errno.
//
// This is equivalent to invoking Readdir.Next over the given
// Readdir instance until it returns syscall.ENOENT.
//
// # Errors
//
// A zero syscall.Errno is returned when Readdir has been successfully exhausted.
// The below are expected otherwise:
//   - syscall.EBADF: the directory is no longer valid
//   - other error values would signal an issue with fetching the next batch of values.
//
// # Notes
//
//   - Notably, ReaddirAll does not return syscall.ENOENT when there are no more
//     entries to fetch, as this is expected behavior for Readdir, and not
//     an actual error. In this case it returns a zero syscall.Errno.
//   - ReaddirAll does not invoke Readdir.Reset, thus, an exhausted Readdir
//     will produce zero entries. This is expected behavior.
//   - Otherwise, the notes for Readdir.Next apply.
func ReaddirAll(dirs fsapi.Readdir) ([]fsapi.Dirent, syscall.Errno) {
	var dirents []fsapi.Dirent
	for {
		e, errno := dirs.Next()
		if errno == syscall.ENOENT {
			return dirents, 0
		} else if errno != 0 {
			return dirents, errno
		}
		if e == nil {
			break
		}
		dirents = append(dirents, *e)
	}
	return dirents, 0
}
