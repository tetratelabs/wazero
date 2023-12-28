//go:build linux || darwin

package sysfs

import (
	"net"
	"syscall"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

// MSG_PEEK is the constant syscall.MSG_PEEK
const MSG_PEEK = syscall.MSG_PEEK

// newTCPListenerFile is a constructor for a socketapi.TCPSock.
//
// Note: the implementation of socketapi.TCPSock goes straight
// to the syscall layer, bypassing most of the Go library.
// For an alternative approach, consider winTcpListenerFile
// where most APIs are implemented with regular Go std-lib calls.
func newTCPListenerFile(tl *net.TCPListener) socketapi.TCPSock {
	return &tcpListenerFile{tl: tl}
}

var _ socketapi.TCPSock = (*tcpListenerFile)(nil)

type tcpListenerFile struct {
	baseSockFile

	tl       *net.TCPListener
	closed   bool
	nonblock bool
}

// Accept implements the same method as documented on socketapi.TCPSock
func (f *tcpListenerFile) Accept() (socketapi.TCPConn, sys.Errno) {
	// Ensure we have an incoming connection, otherwise return immediately.
	if f.nonblock {
		_, errno := syscallConnControl(f.tl, func(fd uintptr) (int, sys.Errno) {
			if ready, errno := poll(fd, fsapi.POLLIN, 0); !ready || errno != 0 {
				return -1, sys.EAGAIN
			} else {
				return 0, 0
			}
		})
		if errno != 0 {
			return nil, errno
		}
	}

	// Accept normally blocks goroutines, but we
	// made sure that we have an incoming connection,
	// so we should be safe.
	if conn, err := f.tl.Accept(); err != nil {
		return nil, sys.UnwrapOSError(err)
	} else {
		return newTcpConn(conn.(*net.TCPConn)), 0
	}
}

// Close implements the same method as documented on sys.File
func (f *tcpListenerFile) Close() sys.Errno {
	if !f.closed {
		return sys.UnwrapOSError(f.tl.Close())
	}
	return 0
}

// Addr is exposed for testing.
func (f *tcpListenerFile) Addr() *net.TCPAddr {
	return f.tl.Addr().(*net.TCPAddr)
}

// SetNonblock implements the same method as documented on fsapi.File
func (f *tcpListenerFile) SetNonblock(enabled bool) (errno sys.Errno) {
	f.nonblock = enabled
	_, errno = syscallConnControl(f.tl, func(fd uintptr) (int, sys.Errno) {
		return 0, sys.UnwrapOSError(setNonblock(fd, enabled))
	})
	return
}

// IsNonblock implements the same method as documented on fsapi.File
func (f *tcpListenerFile) IsNonblock() bool {
	return f.nonblock
}

// Poll implements the same method as documented on fsapi.File
func (f *tcpListenerFile) Poll(flag fsapi.Pflag, timeoutMillis int32) (ready bool, errno sys.Errno) {
	return false, sys.ENOSYS
}

var _ socketapi.TCPConn = (*tcpConnFile)(nil)

type tcpConnFile struct {
	baseSockFile

	tc *net.TCPConn

	// nonblock is true when the underlying connection is flagged as non-blocking.
	// This ensures that reads and writes return sys.EAGAIN without blocking the caller.
	nonblock bool
	// closed is true when closed was called. This ensures proper sys.EBADF
	closed bool
}

func newTcpConn(tc *net.TCPConn) socketapi.TCPConn {
	return &tcpConnFile{tc: tc}
}

// Read implements the same method as documented on sys.File
func (f *tcpConnFile) Read(buf []byte) (n int, errno sys.Errno) {
	if len(buf) == 0 {
		return 0, 0 // Short-circuit 0-len reads.
	}
	if nonBlockingFileReadSupported && f.IsNonblock() {
		n, errno = syscallConnControl(f.tc, func(fd uintptr) (int, sys.Errno) {
			n, err := syscall.Read(int(fd), buf)
			errno = sys.UnwrapOSError(err)
			errno = fileError(f, f.closed, errno)
			return n, errno
		})
	} else {
		n, errno = read(f.tc, buf)
	}
	if errno != 0 {
		// Defer validation overhead until we've already had an error.
		errno = fileError(f, f.closed, errno)
	}
	return
}

// Write implements the same method as documented on sys.File
func (f *tcpConnFile) Write(buf []byte) (n int, errno sys.Errno) {
	if nonBlockingFileWriteSupported && f.IsNonblock() {
		return syscallConnControl(f.tc, func(fd uintptr) (int, sys.Errno) {
			n, err := syscall.Write(int(fd), buf)
			errno = sys.UnwrapOSError(err)
			errno = fileError(f, f.closed, errno)
			return n, errno
		})
	} else {
		n, errno = write(f.tc, buf)
	}
	if errno != 0 {
		// Defer validation overhead until we've already had an error.
		errno = fileError(f, f.closed, errno)
	}
	return
}

// Recvfrom implements the same method as documented on socketapi.TCPConn
func (f *tcpConnFile) Recvfrom(p []byte, flags int) (n int, errno sys.Errno) {
	if flags != MSG_PEEK {
		errno = sys.EINVAL
		return
	}
	return syscallConnControl(f.tc, func(fd uintptr) (int, sys.Errno) {
		n, _, err := syscall.Recvfrom(int(fd), p, MSG_PEEK)
		errno = sys.UnwrapOSError(err)
		errno = fileError(f, f.closed, errno)
		return n, errno
	})
}

// Shutdown implements the same method as documented on sys.Conn
func (f *tcpConnFile) Shutdown(how int) sys.Errno {
	// FIXME: can userland shutdown listeners?
	var err error
	switch how {
	case syscall.SHUT_RD:
		err = f.tc.CloseRead()
	case syscall.SHUT_WR:
		err = f.tc.CloseWrite()
	case syscall.SHUT_RDWR:
		return f.close()
	default:
		return sys.EINVAL
	}
	return sys.UnwrapOSError(err)
}

// Close implements the same method as documented on sys.File
func (f *tcpConnFile) Close() sys.Errno {
	return f.close()
}

func (f *tcpConnFile) close() sys.Errno {
	if f.closed {
		return 0
	}
	f.closed = true
	return f.Shutdown(syscall.SHUT_RDWR)
}

// SetNonblock implements the same method as documented on fsapi.File
func (f *tcpConnFile) SetNonblock(enabled bool) (errno sys.Errno) {
	f.nonblock = enabled
	_, errno = syscallConnControl(f.tc, func(fd uintptr) (int, sys.Errno) {
		return 0, sys.UnwrapOSError(setNonblock(fd, enabled))
	})
	return
}

// IsNonblock implements the same method as documented on fsapi.File
func (f *tcpConnFile) IsNonblock() bool {
	return f.nonblock
}

// Poll implements the same method as documented on fsapi.File
func (f *tcpConnFile) Poll(flag fsapi.Pflag, timeoutMillis int32) (ready bool, errno sys.Errno) {
	return false, sys.ENOSYS
}

// syscallConnControl extracts a syscall.RawConn from the given syscall.Conn and applies
// the given fn to a file descriptor, returning an integer or a nonzero syscall.Errno on failure.
//
// syscallConnControl streamlines the pattern of extracting the syscall.Rawconn,
// invoking its syscall.RawConn.Control method, then handling properly the errors that may occur
// within fn or returned by syscall.RawConn.Control itself.
func syscallConnControl(conn syscall.Conn, fn func(fd uintptr) (int, sys.Errno)) (n int, errno sys.Errno) {
	syscallConn, err := conn.SyscallConn()
	if err != nil {
		return 0, sys.UnwrapOSError(err)
	}
	// Prioritize the inner errno over Control
	if controlErr := syscallConn.Control(func(fd uintptr) {
		n, errno = fn(fd)
	}); errno == 0 {
		errno = sys.UnwrapOSError(controlErr)
	}
	return
}
