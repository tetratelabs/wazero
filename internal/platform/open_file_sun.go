//go:build illumos || solaris

package platform

import (
	"io/fs"
	"os"
	"syscall"
)

const (
	// See https://github.com/illumos/illumos-gate/blob/edd580643f2cf1434e252cd7779e83182ea84945/usr/src/uts/common/sys/fcntl.h#L90
	O_DIRECTORY = 0x1000000
	O_NOFOLLOW  = syscall.O_NOFOLLOW
	O_NONBLOCK  = syscall.O_NONBLOCK
)

func newOsFile(openPath string, openFlag int, openPerm fs.FileMode, f *os.File) File {
	return newDefaultOsFile(openPath, openFlag, openPerm, f)
}

func openFile(path string, flag int, perm fs.FileMode) (*os.File, syscall.Errno) {
	f, err := os.OpenFile(path, flag, perm)
	return f, UnwrapOSError(err)
}
