//go:build linux

package platform

import (
	"os"
	"syscall"
)

func datasync(f *os.File) syscall.Errno {
	return UnwrapOSError(syscall.Fdatasync(int(f.Fd())))
}
