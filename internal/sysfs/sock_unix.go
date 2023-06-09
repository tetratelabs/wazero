//go:build linux || darwin

package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
)

const MSG_PEEK = syscall.MSG_PEEK

// recvfromPeek exposes syscall.Recvfrom with flag MSG_PEEK on POSIX systems.
func recvfromPeek(f *tcpConnFile, p []byte) (n int, errno syscall.Errno) {
	n, _, recvfromErr := syscall.Recvfrom(int(f.fd), p, MSG_PEEK)
	errno = platform.UnwrapOSError(recvfromErr)
	return n, errno
}
