package writefs

import (
	"io/fs"
	"os"
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

	// Remove is similar to os.Remove, except the path is relative to this file
	// system.
	Remove(path string) error
}

func New(absoluteDir string) FS {
	return writeFS(absoluteDir)
}

type writeFS string

// Open implements fs.FS
func (dir writeFS) Open(name string) (fs.File, error) {
	return dir.OpenFile(name, os.O_RDONLY, 0) // same as os.Open(string)
}

// OpenFile implements FS.OpenFile
func (dir writeFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.OpenFile(string(dir)+"/"+name, flag, perm)
}

// Mkdir implements FS.Mkdir
func (dir writeFS) Mkdir(name string, perm fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	return os.Mkdir(string(dir)+"/"+name, perm)
}

// Remove implements FS.Remove
func (dir writeFS) Remove(path string) error {
	if !fs.ValidPath(path) {
		return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrInvalid}
	}
	return os.Remove(string(dir) + "/" + path)
}
