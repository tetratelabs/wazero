package writefs

import (
	"io/fs"
	"os"
	"path"
	"runtime"
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

// Special-case windows as it is necessary to return expected errors. For
// example, GOOS=js uses a static lookup table for error code names.
const (
	// windowsERROR_ALREADY_EXISTS is a windows error returned by os.Mkdir
	// instead of syscall.EEXIST
	windowsERROR_ALREADY_EXISTS = syscall.Errno(183)

	// windowsERROR_DIRECTORY is a windows error returned by os.Rmdir
	// instead of syscall.ENOTDIR
	windowsERROR_DIRECTORY = syscall.Errno(267)
)

// Mkdir implements FS.Mkdir
func (dir dirFS) Mkdir(name string, perm fs.FileMode) (err error) {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}

	err = os.Mkdir(path.Join(string(dir), name), perm)

	// os.Mkdir wraps the syscall error in a path error
	if runtime.GOOS == "windows" && err != nil {
		if pe, ok := err.(*fs.PathError); ok && pe.Err == windowsERROR_ALREADY_EXISTS {
			pe.Err = syscall.EEXIST // adjust it
		}
	}
	return
}

// Rmdir implements FS.Rmdir
func (dir dirFS) Rmdir(name string) (err error) {
	if !fs.ValidPath(name) {
		return syscall.EINVAL
	}

	err =  syscall.Rmdir(path.Join(string(dir), name))


	if runtime.GOOS == "windows" && err != nil {
		if err == windowsERROR_DIRECTORY {
			err = syscall.ENOTDIR // adjust it
		}
	}
	return
}

// Unlink implements FS.Unlink
func (dir dirFS) Unlink(name string) (err error) {
	if !fs.ValidPath(name) {
		return syscall.EINVAL
	}
	realPath := path.Join(string(dir), name)
	err = syscall.Unlink(realPath)
	if err == nil {
		return
	}

	switch err {
	case syscall.EPERM:
		// double-check as EPERM can mean it is a directory
		if stat, statErr := os.Stat(realPath); statErr == nil && stat.IsDir() {
			err = syscall.EISDIR
		}
	}
	return
}
