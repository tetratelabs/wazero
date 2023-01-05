package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
)

// Adapt returns a read-only FS unless the input is already one.
func Adapt(fs fs.FS) FS {
	if sys, ok := fs.(FS); ok {
		return sys
	}
	return &adapter{fs}
}

type adapter struct {
	fs fs.FS
}

// Open implements the same method as documented on fs.FS
func (ro *adapter) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// Path implements FS.Path
func (ro *adapter) Path() string {
	return "/"
}

// OpenFile implements FS.OpenFile
func (ro *adapter) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	if flag != 0 && flag != os.O_RDONLY {
		return nil, syscall.ENOSYS
	}

	path = cleanPath(path)
	f, err := ro.fs.Open(path)
	if err != nil {
		// wrapped is fine while FS.OpenFile emulates os.OpenFile vs syscall.OpenFile.
		return nil, err
	}
	return maskForReads(f), nil
}

func cleanPath(name string) string {
	if len(name) == 0 {
		return name
	}
	// fs.ValidFile cannot be rooted (start with '/')
	cleaned := name
	if name[0] == '/' {
		cleaned = name[1:]
	}
	cleaned = pathutil.Clean(cleaned) // e.g. "sub/." -> "sub"
	return cleaned
}

// Mkdir implements FS.Mkdir
func (ro *adapter) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (ro *adapter) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (ro *adapter) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (ro *adapter) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (ro *adapter) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
