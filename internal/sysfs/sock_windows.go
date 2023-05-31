//go:build windows

package sysfs

import (
	"syscall"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

// MSG_PEEK is the flag PEEK for syscall.Recvfrom on Windows.
// This constant is not exported on this platform.
const MSG_PEEK = 0x2

// SockRecvPeek exposes syscall.Recvfrom with flag MSG_PEEK on Windows.
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
		n, err = recvfrom(syscall.Handle(fd), p, MSG_PEEK)
	})
	if err != nil {
		return n, platform.UnwrapOSError(err)
	}
	if err2 != nil {
		return n, platform.UnwrapOSError(err2)
	}
	return n, 0
}

var (
	// modws2_32 is WinSock.
	modws2_32 = syscall.NewLazyDLL("ws2_32.dll")
	// procrecvfrom exposes recvfrom from WinSock.
	procrecvfrom = modws2_32.NewProc("recvfrom")
)

// recvfrom exposes the underlying syscall in Windows.
//
// Note: since we are only using this to expose MSG_PEEK,
// we do not need really need all the parameters that are actually
// allowed in WinSock.
// We ignore `from *sockaddr` and `fromlen *int`.
func recvfrom(s syscall.Handle, buf []byte, flags int32) (n int, errno syscall.Errno) {
	var _p0 *byte
	if len(buf) > 0 {
		_p0 = &buf[0]
	}
	r0, _, e1 := syscall.SyscallN(
		procrecvfrom.Addr(),
		uintptr(s),
		uintptr(unsafe.Pointer(_p0)),
		uintptr(len(buf)),
		uintptr(flags),
		0, // from *sockaddr (optional)
		0) // fromlen *int (optional)
	n = int(r0)
	if n == -1 {
		return n, e1
	}
	return
}
