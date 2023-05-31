package sysfs

import (
	"net"
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

// SockAccept exposes syscall.Accept on POSIX systems.
func SockAccept(f fsapi.File) (net.Conn, syscall.Errno) {
	conn, err := f.(*listenerFile).ln.Accept()
	if err != nil {
		return nil, platform.UnwrapOSError(err)
	}
	return conn, 0
}

// SockSetNonblock exposes syscall.SetNonblock on POSIX systems.
func SockSetNonblock(conn net.Conn) syscall.Errno {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return syscall.EINVAL // FIXME: better errno?
	}
	syscallConn, err := tcpConn.SyscallConn()
	if err != nil {
		return platform.UnwrapOSError(err)
	}
	var actualErr error
	// Control does not allow to return an error, but it is blocking;
	// so it is ok to modify the external environment and setting
	// `err` directly.
	err = syscallConn.Control(func(fd uintptr) {
		actualErr = setNonblock(fd, true)
	})
	if actualErr != nil {
		return platform.UnwrapOSError(actualErr)
	}
	if err != nil {
		return platform.UnwrapOSError(err)
	}
	return 0
}

// SockShutdown exposes syscall.Shutdown on POSIX systems.
func SockShutdown(f fsapi.File, how int) syscall.Errno {
	// FIXME: can userland shutdown listeners?
	c, ok := f.(*connFile)
	if !ok {
		return syscall.EBADF // FIXME: better errno?
	}
	conn := c.conn
	var err error
	switch how {
	case syscall.SHUT_RD:
		err = conn.CloseRead()
	case syscall.SHUT_WR:
		err = conn.CloseWrite()
	case syscall.SHUT_RDWR:
		err = conn.Close()
	default:
		return syscall.EINVAL
	}
	return platform.UnwrapOSError(err)
}
