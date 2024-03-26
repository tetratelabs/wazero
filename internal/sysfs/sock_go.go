//go:build !tinygo

package sysfs

import (
	"net"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

// Accept implements the same method as documented on socketapi.TCPSock
func (f *tcpListenerFile) Accept() (socketapi.TCPConn, experimentalsys.Errno) {
	// Ensure we have an incoming connection, otherwise return immediately.
	if f.nonblock {
		if ready, errno := _pollSock(f.tl, fsapi.POLLIN, 0); !ready || errno != 0 {
			return nil, experimentalsys.EAGAIN
		}
	}

	// Accept normally blocks goroutines, but we
	// made sure that we have an incoming connection,
	// so we should be safe.
	if conn, err := f.tl.Accept(); err != nil {
		return nil, experimentalsys.UnwrapOSError(err)
	} else {
		return newTcpConn(conn.(*net.TCPConn)), 0
	}
}

// SetNonblock implements the same method as documented on fsapi.File
func (f *tcpListenerFile) SetNonblock(enabled bool) (errno experimentalsys.Errno) {
	f.nonblock = enabled
	_, errno = syscallConnControl(f.tl, func(fd uintptr) (int, experimentalsys.Errno) {
		return 0, setNonblockSocket(fd, enabled)
	})
	return
}

// Shutdown implements the same method as documented on experimentalsys.Conn
func (f *tcpConnFile) Shutdown(how int) experimentalsys.Errno {
	// FIXME: can userland shutdown listeners?
	var err error
	switch how {
	case socketapi.SHUT_RD:
		err = f.tc.CloseRead()
	case socketapi.SHUT_WR:
		err = f.tc.CloseWrite()
	case socketapi.SHUT_RDWR:
		return f.close()
	default:
		return experimentalsys.EINVAL
	}
	return experimentalsys.UnwrapOSError(err)
}
