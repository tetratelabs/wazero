//go:build linux || darwin

package sysfs

import (
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
)

type Sysfd uintptr

const MSG_PEEK = syscall.MSG_PEEK

// recvfromPeek exposes syscall.Recvfrom with flag MSG_PEEK on POSIX systems.
func recvfromPeek(fd Sysfd, p []byte) (n int, errno syscall.Errno) {
	n, _, recvfromErr := syscall.Recvfrom(int(fd), p, MSG_PEEK)
	errno = platform.UnwrapOSError(recvfromErr)
	return n, errno
}

func getSysfd(conn *os.File) Sysfd {
	fd := conn.Fd()
	ffd, err := syscall.Dup(int(fd))
	if err != nil {
		panic(err)
	}
	return Sysfd(ffd)
}

func syscallAccept(fd Sysfd) (Sysfd, syscall.Errno) {
	nfd, _, err := syscall.Accept(int(fd))
	return Sysfd(nfd), platform.UnwrapOSError(err)
}

func syscallClose(fd Sysfd) error {
	return platform.UnwrapOSError(syscall.Close(int(fd)))
}

func syscallRead(fd Sysfd, buf []byte) (n int, errno syscall.Errno) {
	n, err := syscall.Read(int(fd), buf)
	if err != nil {
		// Defer validation overhead until we've already had an error.
		errno = platform.UnwrapOSError(err)
	}
	return n, errno
}

func syscallWrite(fd Sysfd, buf []byte) (n int, errno syscall.Errno) {
	n, err := syscall.Write(int(fd), buf)
	if err != nil {
		// Defer validation overhead until we've already had an error.
		errno = platform.UnwrapOSError(err)
	}
	return n, errno
}

func syscallShutdown(fd Sysfd, how int) syscall.Errno {
	return platform.UnwrapOSError(syscall.Shutdown(int(fd), how))
}
