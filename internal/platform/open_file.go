//go:build !windows && !js && !illumos && !solaris

package platform

import (
	"io/fs"
	"os"
	"syscall"
)

// Simple aliases to constants in the syscall package for portability with
// platforms which do not have them (e.g. windows)
const (
	O_DIRECTORY = syscall.O_DIRECTORY
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_NONBLOCK  = syscall.O_NONBLOCK
)

func newOsFile(openPath string, openFlag int, openPerm fs.FileMode, f *os.File) File {
	return newDefaultOsFile(openPath, openFlag, openPerm, f)
}

// OpenFile is like os.OpenFile except it returns syscall.Errno. A zero
// syscall.Errno is success.
func openFile(path string, flag int, perm fs.FileMode) (*os.File, syscall.Errno) {
	f, err := os.OpenFile(path, flag, perm)
	// Note: This does not return a platform.File because sysfs.FS that returns
	// one may want to hide the real OS path. For example, this is needed for
	// pre-opens.
	return f, UnwrapOSError(err)
}
