package syscallfs

import (
	"io"
	"io/fs"
	"os"
)

// FS is a writeable fs.FS bridge backed by syscall functions needed for ABI
// including WASI and runtime.GOOS=js.
//
// Any unsupported method should return syscall.ENOSYS.
//
// See https://github.com/golang/go/issues/45757
type FS interface {
	// Path is the name of the path the guest should use this filesystem for,
	// or root ("/") if unknown.
	//
	// This value allows the guest to avoid making file-system calls when they
	// won't succeed. e.g. if "/tmp" is returned and the guest requests
	// "/etc/passwd". This approach is used in compilers that use WASI
	// pre-opens.
	//
	// # Notes
	//   - Go compiled with runtime.GOOS=js do not pay attention to this value.
	//     Hence, you need to normalize the filesystem with NewRootFS to ensure
	//     paths requested resolve as expected.
	//   - Working directories are typically tracked in wasm, though possible
	//     some relative paths are requested. For example, TinyGo may attempt
	//     to resolve a path "../.." in unit tests.
	//   - Zig uses the first path name it sees as the initial working
	//     directory of the process.
	Path() string

	// Open is only defined to match the signature of fs.FS until we remove it.
	// Once we are done bridging, we will remove this function. Meanwhile,
	// using it will panic to ensure internal code doesn't depend on it.
	Open(name string) (fs.File, error)

	// OpenFile is similar to os.OpenFile, except the path is relative to this
	// file system.
	OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error)
	// ^^ TODO: Consider syscall.Open, though this implies defining and
	// coercing flags and perms similar to what is done in os.OpenFile.

	// Mkdir is similar to os.Mkdir, except the path is relative to this file
	// system.
	Mkdir(path string, perm fs.FileMode) error
	// ^^ TODO: Consider syscall.Mkdir, though this implies defining and
	// coercing flags and perms similar to what is done in os.Mkdir.

	// Rename is similar to syscall.Rename, except the path is relative to this
	// file system.
	//
	// # Errors
	//
	// The following errors are expected:
	//   - syscall.EINVAL: `from` or `to` is invalid.
	//   - syscall.ENOENT: `from` or `to` don't exist.
	//   - syscall.ENOTDIR: `from` is a directory and `to` exists, but is a file.
	//   - syscall.EISDIR: `from` is a file and `to` exists, but is a directory.
	//
	// # Notes
	//
	//   -  Windows doesn't let you overwrite an existing directory.
	Rename(from, to string) error

	// Rmdir is similar to syscall.Rmdir, except the path is relative to this
	// file system.
	//
	// # Errors
	//
	// The following errors are expected:
	//   - syscall.EINVAL: `path` is invalid.
	//   - syscall.ENOENT: `path` doesn't exist.
	//   - syscall.ENOTDIR: `path` exists, but isn't a directory.
	//   - syscall.ENOTEMPTY: `path` exists, but isn't empty.
	//
	// # Notes
	//
	//   - As of Go 1.19, Windows maps syscall.ENOTDIR to syscall.ENOENT.
	Rmdir(path string) error

	// Unlink is similar to syscall.Unlink, except the path is relative to this
	// file system.
	//
	// The following errors are expected:
	//   - syscall.EINVAL: `path` is invalid.
	//   - syscall.ENOENT: `path` doesn't exist.
	//   - syscall.EISDIR: `path` exists, but is a directory.
	Unlink(path string) error

	// Utimes is similar to syscall.UtimesNano, except the path is relative to
	// this file system.
	//
	// # Errors
	//
	// The following errors are expected:
	//   - syscall.EINVAL: `path` is invalid.
	//   - syscall.ENOENT: `path` doesn't exist
	//
	// # Notes
	//
	//   - To set wall clock time, retrieve it first from sys.Walltime.
	//   - syscall.UtimesNano cannot change the ctime. Also, neither WASI nor
	//     runtime.GOOS=js support changing it. Hence, ctime it is absent here.
	Utimes(path string, atimeNsec, mtimeNsec int64) error
}

// maskForReads masks the file with read-only interfaces used by wazero.
//
// Note: This technique was adapted from similar code in zipkin-go.
func maskForReads(f fs.File) fs.File {
	// The below are the types wazero casts into.
	// Note: os.File implements this even for normal files.
	d, i0 := f.(fs.ReadDirFile)
	ra, i1 := f.(io.ReaderAt)
	s, i2 := f.(io.Seeker)

	// Wrap any combination of the types above.
	switch {
	case !i0 && !i1 && !i2: // 0, 0, 0
		return struct{ fs.File }{f}
	case !i0 && !i1 && i2: // 0, 0, 1
		return struct {
			fs.File
			io.Seeker
		}{f, s}
	case !i0 && i1 && !i2: // 0, 1, 0
		return struct {
			fs.File
			io.ReaderAt
		}{f, ra}
	case !i0 && i1 && i2: // 0, 1, 1
		return struct {
			fs.File
			io.ReaderAt
			io.Seeker
		}{f, ra, s}
	case i0 && !i1 && !i2: // 1, 0, 0
		return struct {
			fs.ReadDirFile
		}{d}
	case i0 && !i1 && i2: // 1, 0, 1
		return struct {
			fs.ReadDirFile
			io.Seeker
		}{d, s}
	case i0 && i1 && !i2: // 1, 1, 0
		return struct {
			fs.ReadDirFile
			io.ReaderAt
		}{d, ra}
	case i0 && i1 && i2: // 1, 1, 1
		return struct {
			fs.ReadDirFile
			io.ReaderAt
			io.Seeker
		}{d, ra, s}
	default:
		panic("BUG: unhandled pattern")
	}
}

// StatPath is a convenience that calls FS.OpenFile until there is a stat
// method.
func StatPath(fs FS, path string) (fs.FileInfo, error) {
	f, err := fs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}
