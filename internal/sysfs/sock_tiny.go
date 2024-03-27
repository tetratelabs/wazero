//go:build tinygo

package sysfs

import (
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

// Accept implements the same method as documented on socketapi.TCPSock
func (f *tcpListenerFile) Accept() (socketapi.TCPConn, experimentalsys.Errno) {
	panic("TCPSock.Accept is not implemented for TinyGo")
}

// Shutdown implements the same method as documented on experimentalsys.Conn
func (f *tcpConnFile) Shutdown(how int) experimentalsys.Errno {
	// FIXME: can userland shutdown listeners?
	var err error
	switch how {
	case socketapi.SHUT_RD:
		err = f.tc.Close()
	case socketapi.SHUT_WR:
		err = f.tc.Close()
	case socketapi.SHUT_RDWR:
		return f.close()
	default:
		return experimentalsys.EINVAL
	}
	return experimentalsys.UnwrapOSError(err)
}
