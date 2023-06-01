//go:build unix || linux || darwin

package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

// SockRecvPeek exposes syscall.Recvfrom with flag MSG_PEEK on POSIX systems.
func SockRecvPeek(f fsapi.File, p []byte) (n int, errno syscall.Errno) {
	c, ok := f.(*connFile)
	if !ok {
		return 0, syscall.EBADF // FIXME: better errno?
	}
	syscallConn, err := c.conn.SyscallConn()
	if err != nil {
		return 0, platform.UnwrapOSError(err)
	}

	// Prioritize the error from Recvfrom over Control
	if controlErr := syscallConn.Control(func(fd uintptr) {
		var recvfromErr error
		n, _, recvfromErr = syscall.Recvfrom(int(fd), p, syscall.MSG_PEEK)
		errno = platform.UnwrapOSError(recvfromErr)
	}); errno == 0 {
		errno = platform.UnwrapOSError(controlErr)
	}
	return
}
