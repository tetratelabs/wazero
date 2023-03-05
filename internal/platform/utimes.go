package platform

import (
	"io/fs"
	"syscall"
)

// UtimesNano is like syscall.UtimesNano. This returns syscall.ENOENT if the
// path doesn't exist.
//
// Note: This is like the function `utimensat` with `AT_FDCWD` in POSIX.
// See https://pubs.opengroup.org/onlinepubs/9699919799/functions/futimens.html
func UtimesNano(path string, atimeNsec, mtimeNsec int64) error {
	err := syscall.UtimesNano(path, []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	})
	return UnwrapOSError(err)
}

// UtimesNanoFile is like syscall.Futimes, but for nanosecond precision and
// fs.File instead of a file descriptor. This returns syscall.EBADF if the file
// or directory was closed, or syscall.EPERM if the file wasn't opened with
// permission to update its utimes. On syscall.EPERM, or on syscall.ENOSYS, use
// UtimesNano with the original path.
//
// Note: This is like the function `futimens` in POSIX.
// See https://pubs.opengroup.org/onlinepubs/9699919799/functions/futimens.html
func UtimesNanoFile(f fs.File, atimeNsec, mtimeNsec int64) error {
	if f, ok := f.(fdFile); ok {
		err := futimens(f.Fd(), atimeNsec, mtimeNsec)
		return UnwrapOSError(err)
	}
	return syscall.ENOSYS
}
