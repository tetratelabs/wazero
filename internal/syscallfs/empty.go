package syscallfs

import (
	"fmt"
	"io/fs"
	"syscall"
)

// EmptyFS is an FS that returns syscall.ENOENT for all read functions, and
// syscall.ENOSYS otherwise.
var EmptyFS FS = empty{}

type empty struct{}

// Open implements the same method as documented on fs.FS
func (empty) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (empty) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (empty) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.ENOENT}
}

// Mkdir implements FS.Mkdir
func (empty) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (empty) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (empty) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (empty) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (empty) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
