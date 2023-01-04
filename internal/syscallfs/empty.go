package syscallfs

import (
	"fmt"
	"io/fs"
	"syscall"
)

// EmptyFS is an FS that returns syscall.ENOENT for all read functions, and
// syscall.ENOSYS otherwise.
var EmptyFS FS = unsupported{}

type unsupported struct{}

// Open implements the same method as documented on fs.FS
func (unsupported) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// OpenFile implements FS.OpenFile
func (unsupported) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.ENOENT}
}

// Mkdir implements FS.Mkdir
func (unsupported) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (unsupported) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (unsupported) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (unsupported) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (unsupported) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
