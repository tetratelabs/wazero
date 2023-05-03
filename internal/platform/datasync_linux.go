//go:build linux

package platform

import (
	"io/fs"
	"syscall"
)

func datasync(f fs.File) syscall.Errno {
	if fd, ok := f.(fdFile); ok {
		return UnwrapOSError(syscall.Fdatasync(int(fd.Fd())))
	}

	// Attempt to sync everything, even if we only need to sync the data.
	return sync(f)
}
