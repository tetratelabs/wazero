package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func NewReadFS(fs FS) FS {
	if _, ok := fs.(*readFS); ok {
		return fs
	}
	return &readFS{fs}
}

type readFS struct{ fs FS }

// Open implements the same method as documented on fs.FS
func (ro *readFS) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// OpenFile implements FS.OpenFile
func (ro *readFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	if flag == 0 || flag == os.O_RDONLY {
		return ro.fs.OpenFile(name, flag, perm)
	}
	return nil, syscall.ENOSYS
}

// Mkdir implements FS.Mkdir
func (ro *readFS) Mkdir(name string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (ro *readFS) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (ro *readFS) Rmdir(name string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (ro *readFS) Unlink(name string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (ro *readFS) Utimes(name string, atimeNsec, mtimeNsec int64) error {
	return ro.fs.Utimes(name, atimeNsec, mtimeNsec)
}
