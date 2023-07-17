//go:build unix || darwin || linux

package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/experimental/sys"
)

const nonBlockingFileIoSupported = true

// readFd exposes syscall.Read.
func readFd(fd uintptr, buf []byte) (int, sys.Errno) {
	if len(buf) == 0 {
		return 0, 0 // Short-circuit 0-len reads.
	}
	n, err := syscall.Read(int(fd), buf)
	errno := sys.UnwrapOSError(err)
	return n, errno
}
