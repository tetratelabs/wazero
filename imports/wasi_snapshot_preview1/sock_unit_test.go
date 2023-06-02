package wasi_snapshot_preview1

import (
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/sock"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
)

func Test_getExtendedWasiFiletype(t *testing.T) {
	s := testSock{}
	ftype := getExtendedWasiFiletype(s, os.ModeIrregular)
	require.Equal(t, wasip1.FILETYPE_SOCKET_STREAM, ftype)

	c := testConn{}
	ftype = getExtendedWasiFiletype(c, os.ModeIrregular)
	require.Equal(t, wasip1.FILETYPE_SOCKET_STREAM, ftype)
}

type testSock struct {
	fsapi.UnimplementedFile
}

func (t testSock) Accept() (sock.TCPConn, syscall.Errno) {
	panic("no-op")
}

type testConn struct {
	fsapi.UnimplementedFile
}

func (t testConn) Recvfrom([]byte, int) (n int, errno syscall.Errno) {
	panic("no-op")
}

func (t testConn) Shutdown(int) syscall.Errno {
	panic("no-op")
}
