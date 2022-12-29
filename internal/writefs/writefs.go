package writefs

import (
	"io/fs"
	"os"
	"path"
	"syscall"
)

// FS is a fs.FS which can also create new files or directories.
//
// Any unsupported method should return syscall.ENOSYS.
//
// See https://github.com/golang/go/issues/45757
type FS interface {
	fs.FS

	// OpenFile is similar to os.OpenFile, except the path is relative to this
	// file system.
	OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error)

	// Mkdir is similar to os.Mkdir, except the path is relative to this file
	// system.
	Mkdir(name string, perm fs.FileMode) error

	// Rmdir is similar to syscall.Rmdir, except the path is relative to this
	// file system.
	//
	// # Errors
	//
	// The following errors are expected:
	//   - syscall.EINVAL: `path` is invalid.
	//   - syscall.ENOENT: `path` doesn't exist.
	//   - syscall.ENOTDIR: `path` exists, but isn't a directory.
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

func DirFS(dir string) FS {
	return dirFS(dir)
}

type dirFS string

// Open implements the same method as documented on fs.FS
func (dir dirFS) Open(name string) (fs.File, error) {
	return dir.OpenFile(name, os.O_RDONLY, 0) // same as os.Open(string)
}

// OpenFile implements FS.OpenFile
func (dir dirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.OpenFile(path.Join(string(dir), name), flag, perm)
}

// Mkdir implements FS.Mkdir
func (dir dirFS) Mkdir(name string, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}

	err := os.Mkdir(path.Join(string(dir), name), perm)

	return adjustMkdirError(err)
}

// Rmdir implements FS.Rmdir
func (dir dirFS) Rmdir(name string) error {
	if !fs.ValidPath(name) {
		return syscall.EINVAL
	}

	err := syscall.Rmdir(path.Join(string(dir), name))

	return adjustRmdirError(err)
}

// Unlink implements FS.Unlink
func (dir dirFS) Unlink(name string) error {
	if !fs.ValidPath(name) {
		return syscall.EINVAL
	}

	err := syscall.Unlink(path.Join(string(dir), name))

	return adjustUnlinkError(err)
}
