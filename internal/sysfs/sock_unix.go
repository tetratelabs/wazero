//go:build unix || linux || darwin

package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

// SockRecvPeek exposes syscall.Recvfrom with flag MSG_PEEK on POSIX systems.
func SockRecvPeek(f fsapi.File, p []byte) (int, syscall.Errno) {
	c, ok := f.(*connFile)
	if !ok {
		return -1, syscall.EBADF // FIXME: better errno?
	}
	syscallConn, err := c.conn.SyscallConn()
	if err != nil {
		return 0, platform.UnwrapOSError(err)
	}
	n := 0
	// Control does not allow to return an error, but it is blocking;
	// so it is ok to modify the external environment and setting
	// `err` directly.
	err2 := syscallConn.Control(func(fd uintptr) {
		n, _, err = syscall.Recvfrom(int(fd), p, syscall.MSG_PEEK)
	})
	if err != nil {
		return n, platform.UnwrapOSError(err)
	}
	if err2 != nil {
		return n, platform.UnwrapOSError(err2)
	}
	return n, 0
}
