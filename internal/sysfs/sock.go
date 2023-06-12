package sysfs

import (
	"net"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

// NewTCPListenerFile creates a socketapi.TCPSock for a given *net.TCPListener.
func NewTCPListenerFile(tl *net.TCPListener) socketapi.TCPSock {
	return newTCPListenerFile(tl)
}

// baseSockFile implements base behavior for all TCPSock, TCPConn files,
// regardless the platform.
type baseSockFile struct {
	fsapi.UnimplementedFile
}

var (
	_ fsapi.File        = (*baseSockFile)(nil)
	_ socketapi.TCPSock = (*baseSockFile)(nil)
	_ socketapi.TCPConn = (*baseSockFile)(nil)
)

// IsDir implements the same method as documented on File.IsDir
func (*baseSockFile) IsDir() (bool, syscall.Errno) {
	// We need to override this method because WASI-libc prestats the FD
	// and the default impl returns ENOSYS otherwise.
	return false, 0
}

// Stat implements the same method as documented on File.Stat
func (f *baseSockFile) Stat() (fs fsapi.Stat_t, errno syscall.Errno) {
	// The mode is not really important, but it should be neither a regular file nor a directory.
	fs.Mode = os.ModeIrregular
	return
}

// Accept implements the same method as documented on socketapi.TCPSock
func (f *baseSockFile) Accept() (socketapi.TCPConn, syscall.Errno) {
	return nil, syscall.ENOSYS
}

// Recvfrom implements the same method as documented on socketapi.TCPConn
func (f *baseSockFile) Recvfrom(p []byte, flags int) (n int, errno syscall.Errno) {
	return -1, syscall.ENOSYS
}

// Shutdown implements the same method as documented on fsapi.Conn
func (f *baseSockFile) Shutdown(how int) syscall.Errno {
	return syscall.ENOSYS
}
