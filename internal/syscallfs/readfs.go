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
func (r *readFS) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (r *readFS) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (r *readFS) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	if flag == 0 || flag == os.O_RDONLY {
		return r.fs.OpenFile(path, flag, perm)
	}
	return nil, syscall.ENOSYS
}

// Mkdir implements FS.Mkdir
func (r *readFS) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (r *readFS) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (r *readFS) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (r *readFS) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (r *readFS) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
