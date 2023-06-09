package sysfs

import (
	"net"
	"syscall"
	"testing"
	"time"

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
	errno := syscall.Errno(0)
	for {
		_, errno = file.Write([]byte("wazero"))
		if errno != syscall.EAGAIN {
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
	errno := syscall.Errno(0)
	file := newTcpConn(conn.(*net.TCPConn))
	for {
		_, errno = file.Read(bytes)
		if errno != syscall.EAGAIN {
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
