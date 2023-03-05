package platform

import (
	"io/fs"
	"syscall"
)

// UtimesNano is like syscall.UtimesNano. This returns syscall.ENOENT if the
// path doesn't exist.
//
// See https://linux.die.net/man/3/futimens
func UtimesNano(path string, atimeNsec, mtimeNsec int64) error {
	err := syscall.UtimesNano(path, []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	})
	return UnwrapOSError(err)
}

// UtimesNanoFile is like syscall.Futimes, but for nanosecond precision and
// fs.File instead of a file descriptor. This returns syscall.EBADF if the file
// or directory was closed.
//
// See https://linux.die.net/man/3/futimens
func UtimesNanoFile(f fs.File, atimeNsec, mtimeNsec int64) error {
	if f, ok := f.(fdFile); ok {
		err := futimens(f.Fd(), atimeNsec, mtimeNsec)
		return UnwrapOSError(err)
	}
	return syscall.ENOSYS
}
