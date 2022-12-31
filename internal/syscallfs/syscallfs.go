package syscallfs

import (
	"io/fs"
)

// FS is a writeable fs.FS bridge backed by syscall functions needed for ABI
// including WASI and runtime.GOOS=js.
//
// Any unsupported method should return syscall.ENOSYS.
//
// See https://github.com/golang/go/issues/45757
type FS interface {
	// Open is only defined to match the signature of fs.FS until we remove it.
	// Once we are done bridging, we will remove this function. Meanwhile,
	// using it will panic to ensure internal code doesn't depend on it.
	Open(name string) (fs.File, error)

	// OpenFile is similar to os.OpenFile, except the path is relative to this
	// file system.
	OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error)
	// ^^ TODO: switch to syscall.X, not os.X

	// Mkdir is similar to os.Mkdir, except the path is relative to this file
	// system.
	Mkdir(name string, perm fs.FileMode) error
	// ^^ TODO: switch to syscall.X, not os.X

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
	Utimes(path string, atimeSec, atimeNsec, mtimeSec, mtimeNsec int64) error

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
}
