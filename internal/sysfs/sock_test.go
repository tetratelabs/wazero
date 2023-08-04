package sysfs

import (
	"net"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestTcpConnFile_Write(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	file := newTcpConn(tcp)
	errno := sys.Errno(0)
	// Ensure we don't interrupt until we get a non-zero errno,
	// and we retry on EAGAIN (i.e. when nonblocking is true).
	for {
		_, errno = file.Write([]byte("wazero"))
		if errno != sys.EAGAIN {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Zero(t, errno)

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	bytes := make([]byte, 4)

	n, err := conn.Read(bytes)
	require.NoError(t, err)
	require.NotEqual(t, 0, n)

	require.Equal(t, "waze", string(bytes))
}

func TestTcpConnFile_Read(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	n, err := tcp.Write([]byte("wazero"))
	require.NoError(t, err)
	require.NotEqual(t, 0, n)

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	bytes := make([]byte, 4)

	require.NoError(t, err)
	errno := sys.Errno(0)
	file := newTcpConn(conn.(*net.TCPConn))
	// Ensure we don't interrupt until we get a non-zero errno,
	// and we retry on EAGAIN (i.e. when nonblocking is true).
	for {
		_, errno = file.Read(bytes)
		if errno != sys.EAGAIN {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Zero(t, errno)
	require.NoError(t, err)
	require.Equal(t, "waze", string(bytes))
}

func TestTcpConnFile_Stat(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	conn, err := listen.Accept()
	require.NoError(t, err)
	defer conn.Close()

	file := newTcpConn(tcp)
	_, errno := file.Stat()
	require.Zero(t, errno, "Stat should not fail")
}

func TestTcpConnFile_SetNonblock(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listen.Close()

	lf := newTCPListenerFile(listen.(*net.TCPListener))

	tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
	require.NoError(t, err)
	tcp, err := net.DialTCP("tcp", nil, tcpAddr)
	require.NoError(t, err)
	defer tcp.Close() //nolint

	errno := lf.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, lf.IsNonblock())

	conn, errno := lf.Accept()
	require.EqualErrno(t, 0, errno)
	defer conn.Close()

	file := newTcpConn(tcp)
	errno = file.SetNonblock(true)
	require.EqualErrno(t, 0, errno)
	require.True(t, file.IsNonblock())
}
