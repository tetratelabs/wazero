package syscallfs

import (
	"fmt"
	"io/fs"
	"os"
	pathutil "path"
	"syscall"
)

// Adapt adapts the input to FS unless it is already one. NewDirFS should be
// used instead, if the input is os.DirFS.
//
// Note: This performs no flag verification on FS.OpenFile. fs.FS cannot read
// flags as there is no parameter to pass them through with. Moreover, fs.FS
// documentation does not require the file to be present. In summary, we can't
// enforce flag behavior.
func Adapt(guestDir string, fs fs.FS) FS {
	if sys, ok := fs.(FS); ok {
		return sys
	}
	return &adapter{guestDir, fs}
}

type adapter struct {
	guestDir string
	fs       fs.FS
}

// Open implements the same method as documented on fs.FS
func (a *adapter) Open(name string) (fs.File, error) {
	panic(fmt.Errorf("unexpected to call fs.FS.Open(%s)", name))
}

// GuestDir implements FS.GuestDir
func (a *adapter) GuestDir() string {
	return a.guestDir
}

// OpenFile implements FS.OpenFile
func (a *adapter) OpenFile(path string, flag int, perm fs.FileMode) (fs.File, error) {
	path = cleanPath(path)
	f, err := a.fs.Open(path)

	if err != nil {
		if pe, ok := err.(*fs.PathError); ok {
			switch pe.Err {
			case os.ErrInvalid:
				pe.Err = syscall.EINVAL // adjust it
			case os.ErrNotExist:
				pe.Err = syscall.ENOENT // adjust it
			}
		}
		return nil, err
	} else if osF, ok := f.(*os.File); ok {
		// If this is an OS file, it has same portability issues as dirFS.
		return maybeWrapFile(osF), nil
	}
	return f, nil
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
func (a *adapter) Mkdir(path string, perm fs.FileMode) error {
	return syscall.ENOSYS
}

// Rename implements FS.Rename
func (a *adapter) Rename(from, to string) error {
	return syscall.ENOSYS
}

// Rmdir implements FS.Rmdir
func (a *adapter) Rmdir(path string) error {
	return syscall.ENOSYS
}

// Unlink implements FS.Unlink
func (a *adapter) Unlink(path string) error {
	return syscall.ENOSYS
}

// Utimes implements FS.Utimes
func (a *adapter) Utimes(path string, atimeNsec, mtimeNsec int64) error {
	return syscall.ENOSYS
}
