//go:build !windows

package sysfs

import (
	"net"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/platform"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

func NewTCPListenerFile(tl *net.TCPListener) socketapi.TCPSock {
	conn, err := tl.File()
	if err != nil {
		panic(err)
	}
	sysfd := getSysfd(conn)
	return &tcpListenerFile{fd: sysfd, tl: tl}
}

var _ socketapi.TCPSock = (*tcpListenerFile)(nil)

type tcpListenerFile struct {
	fsapi.UnimplementedFile

	fd Sysfd
	tl *net.TCPListener
}

// Accept implements the same method as documented on socketapi.TCPSock
func (f *tcpListenerFile) Accept() (socketapi.TCPConn, syscall.Errno) {
	nfd, err := syscallAccept(f.fd)
	if err != 0 {
		return nil, err
	}
	return &tcpConnFile{fd: nfd}, 0
}

// IsDir implements the same method as documented on File.IsDir
func (*tcpListenerFile) IsDir() (bool, syscall.Errno) {
	// We need to override this method because WASI-libc prestats the FD
	// and the default impl returns ENOSYS otherwise.
	return false, 0
}

// Stat implements the same method as documented on File.Stat
func (f *tcpListenerFile) Stat() (fs fsapi.Stat_t, errno syscall.Errno) {
	// The mode is not really important, but it should be neither a regular file nor a directory.
	fs.Mode = os.ModeIrregular
	return
}

// SetNonblock implements the same method as documented on fsapi.File
func (f *tcpListenerFile) SetNonblock(enabled bool) syscall.Errno {
	return platform.UnwrapOSError(setNonblock(f.fd, enabled))
}

// Close implements the same method as documented on fsapi.File
func (f *tcpListenerFile) Close() syscall.Errno {
	return platform.UnwrapOSError(syscallClose(f.fd))
}

// Addr is exposed for testing.
func (f *tcpListenerFile) Addr() *net.TCPAddr {
	return f.tl.Addr().(*net.TCPAddr)
}

var _ socketapi.TCPConn = (*tcpConnFile)(nil)

type tcpConnFile struct {
	fsapi.UnimplementedFile

	fd Sysfd

	// closed is true when closed was called. This ensures proper syscall.EBADF
	closed bool
}

func newTcpConn(tc *net.TCPConn) socketapi.TCPConn {
	f, err := tc.File()
	if err != nil {
		panic(err)
	}
	return &tcpConnFile{fd: Sysfd(f.Fd())}
}

// IsDir implements the same method as documented on File.IsDir
func (*tcpConnFile) IsDir() (bool, syscall.Errno) {
	// We need to override this method because WASI-libc prestats the FD
	// and the default impl returns ENOSYS otherwise.
	return false, 0
}

// Stat implements the same method as documented on File.Stat
func (f *tcpConnFile) Stat() (fs fsapi.Stat_t, errno syscall.Errno) {
	// The mode is not really important, but it should be neither a regular file nor a directory.
	fs.Mode = os.ModeIrregular
	return
}

// SetNonblock implements the same method as documented on fsapi.File
func (f *tcpConnFile) SetNonblock(enabled bool) (errno syscall.Errno) {
	return platform.UnwrapOSError(setNonblock(f.fd, enabled))
}

// Read implements the same method as documented on fsapi.File
func (f *tcpConnFile) Read(buf []byte) (n int, errno syscall.Errno) {
	n, errno = syscallRead(f.fd, buf)
	if errno != 0 {
		// Defer validation overhead until we've already had an error.
		errno = fileError(f, f.closed, errno)
	}
	return n, errno
}

// Write implements the same method as documented on fsapi.File
func (f *tcpConnFile) Write(buf []byte) (n int, errno syscall.Errno) {
	n, errno = syscallWrite(f.fd, buf)
	if errno != 0 {
		// Defer validation overhead until we've alwritey had an error.
		errno = fileError(f, f.closed, errno)
	}
	return n, errno
}

// Recvfrom implements the same method as documented on socketapi.TCPConn
func (f *tcpConnFile) Recvfrom(p []byte, flags int) (n int, errno syscall.Errno) {
	if flags != MSG_PEEK {
		errno = syscall.EINVAL
		return
	}
	return recvfromPeek(f.fd, p)
}

// Shutdown implements the same method as documented on fsapi.Conn
func (f *tcpConnFile) Shutdown(how int) syscall.Errno {
	// FIXME: can userland shutdown listeners?
	var err error
	switch how {
	case syscall.SHUT_RD, syscall.SHUT_WR:
		err = syscallShutdown(f.fd, how)
	case syscall.SHUT_RDWR:
		return f.close()
	default:
		return syscall.EINVAL
	}
	return platform.UnwrapOSError(err)
}

// Close implements the same method as documented on fsapi.File
func (f *tcpConnFile) Close() syscall.Errno {
	return f.close()
}

func (f *tcpConnFile) close() syscall.Errno {
	if f.closed {
		return 0
	}
	f.closed = true
	return platform.UnwrapOSError(syscallShutdown(f.fd, syscall.SHUT_RDWR))
}
