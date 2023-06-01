package wasi_snapshot_preview1_test

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalnet "github.com/tetratelabs/wazero/experimental/net"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_sockAccept(t *testing.T) {
	tests := []struct {
		name          string
		flags         uint16
		expectedErrno wasip1.Errno
		expectedLog   string
		body          func(mod api.Module, log *bytes.Buffer, fd, connFd uintptr, tcp *net.TCPConn)
	}{
		{
			name:          "sock_accept",
			flags:         0,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=%[2]d,result.fd=128)
<== errno=ESUCCESS
`,
		},
		{
			name:  "sock_accept (nonblock)",
			flags: wasip1.FD_NONBLOCK,
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=%[2]d,result.fd=128)
<== errno=ESUCCESS
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := experimentalnet.WithConfig(testCtx, experimentalnet.NewConfig().WithTCPListener("127.0.0.1", 0))

			mod, r, log := requireProxyModuleWithContext(ctx, t, wazero.NewModuleConfig())
			defer r.Close(testCtx)

			// Dial the socket so that a call to accept doesn't hang.
			tcpAddr := requireTCPListenerAddr(t, mod)
			tcp, err := net.DialTCP("tcp", nil, tcpAddr)
			require.NoError(t, err)
			defer tcp.Close() //nolint

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.SockAcceptName, uint64(sys.FdPreopen), uint64(tc.flags), 128)
			connFd, _ := mod.Memory().ReadUint32Le(128)
			require.NotEqual(t, 0, connFd)

			require.Equal(t, fmt.Sprintf(tc.expectedLog, sys.FdPreopen, tc.flags, connFd), "\n"+log.String())
		})
	}
}

func Test_sockShutdown(t *testing.T) {
	tests := []struct {
		name          string
		flags         uint16
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "sock_shutdown",
			flags:         wasip1.SD_WR | wasip1.SD_RD,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_shutdown(fd=%[2]d,how=3)
<== errno=ESUCCESS
`,
		},
		{
			name:          "sock_shutdown: fail with no flags",
			flags:         0,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_shutdown(fd=%[2]d,how=0)
<== errno=EINVAL
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := experimentalnet.WithConfig(testCtx, experimentalnet.NewConfig().WithTCPListener("127.0.0.1", 0))

			mod, r, log := requireProxyModuleWithContext(ctx, t, wazero.NewModuleConfig())
			defer r.Close(testCtx)

			// Dial the socket so that a call to accept doesn't hang.
			tcpAddr := requireTCPListenerAddr(t, mod)
			tcp, err := net.DialTCP("tcp", nil, tcpAddr)
			require.NoError(t, err)
			defer tcp.Close() //nolint

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.SockAcceptName, uint64(sys.FdPreopen), uint64(0), 128)
			connFd, _ := mod.Memory().ReadUint32Le(128)
			require.NotEqual(t, 0, connFd)

			// End of setup. Perform the test.
			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.SockShutdownName, uint64(connFd), uint64(tc.flags))

			require.Equal(t, fmt.Sprintf(tc.expectedLog, sys.FdPreopen, connFd), "\n"+log.String())
		})
	}
}

func Test_sockRecv(t *testing.T) {
	tests := []struct {
		name           string
		funcName       string
		flags          uint32
		expectedErrno  wasip1.Errno
		expectedLog    string
		initialMemory  []byte
		iovsCount      uint64
		expectedMemory []byte
	}{
		{
			name:      "sock_recv",
			iovsCount: 3,
			initialMemory: []byte{
				'?',         // `iovs` is after this
				26, 0, 0, 0, // = iovs[0].offset
				4, 0, 0, 0, // = iovs[0].length
				31, 0, 0, 0, // = iovs[1].offset
				0, 0, 0, 0, // = iovs[1].length == 0 !!
				31, 0, 0, 0, // = iovs[2].offset
				2, 0, 0, 0, // = iovs[2].length
				'?',
			},
			expectedMemory: []byte{
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?',      // iovs[2].offset is after this
				'r', 'o', // iovs[2].length bytes
				'?',                    // resultNread is after this
				6, 0, 0, 0, 0, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				0, 0, 0, 0, // flags
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_recv(fd=%[2]d,ri_data=1,ri_data_count=3,ri_flags=0,result.ro_datalen=34,result.ro_flags=42)
<== errno=ESUCCESS
`,
		},

		{
			name:      "sock_recv (WAITALL)",
			flags:     wasip1.RECV_WAITALL,
			iovsCount: 3,
			initialMemory: []byte{
				'?',         // `iovs` is after this
				26, 0, 0, 0, // = iovs[0].offset
				4, 0, 0, 0, // = iovs[0].length
				31, 0, 0, 0, // = iovs[1].offset
				0, 0, 0, 0, // = iovs[1].length == 0 !!
				31, 0, 0, 0, // = iovs[2].offset
				2, 0, 0, 0, // = iovs[2].length
				'?',
			},
			expectedMemory: []byte{
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?',      // iovs[2].offset is after this
				'r', 'o', // iovs[2].length bytes
				'?',                    // resultNread is after this
				6, 0, 0, 0, 0, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				0, 0, 0, 0, // flags
				'?',
			},

			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_recv(fd=%[2]d,ri_data=1,ri_data_count=3,ri_flags=2,result.ro_datalen=34,result.ro_flags=42)
<== errno=ESUCCESS
`,
		},

		{
			name:      "sock_recv (PEEK)",
			flags:     wasip1.RECV_PEEK,
			iovsCount: 3,
			initialMemory: []byte{
				'?',         // `iovs` is after this
				26, 0, 0, 0, // = iovs[0].offset
				4, 0, 0, 0, // = iovs[0].length
				31, 0, 0, 0, // = iovs[1].offset
				0, 0, 0, 0, // = iovs[1].length == 0 !!
				31, 0, 0, 0, // = iovs[2].offset
				2, 0, 0, 0, // = iovs[2].length
				'?',
			},
			expectedMemory: []byte{
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?', '?', '?', '?', // pad to 34
				4, 0, 0, 0, 0, 0, 0, 0, // result.ro_datalen
				0, 0, 0, 0, // result.ro_flags
				'?',
			},

			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_recv(fd=%[2]d,ri_data=1,ri_data_count=3,ri_flags=1,result.ro_datalen=34,result.ro_flags=42)
<== errno=ESUCCESS
`,
		},
		{
			name:  "sock_recv: fail with unknown flags",
			flags: 42,
			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_recv(fd=%[2]d,ri_data=1,ri_data_count=0,ri_flags=42,result.ro_datalen=34,result.ro_flags=42)
<== errno=ENOTSUP
`,
			expectedErrno: wasip1.ErrnoNotsup,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := experimentalnet.WithConfig(testCtx, experimentalnet.NewConfig().WithTCPListener("127.0.0.1", 0))

			mod, r, log := requireProxyModuleWithContext(ctx, t, wazero.NewModuleConfig())
			defer r.Close(testCtx)

			// Dial the socket so that a call to accept doesn't hang.
			tcpAddr := requireTCPListenerAddr(t, mod)
			tcp, err := net.DialTCP("tcp", nil, tcpAddr)
			require.NoError(t, err)
			defer tcp.Close() //nolint

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.SockAcceptName, uint64(sys.FdPreopen), uint64(0), 128)
			connFd, _ := mod.Memory().ReadUint32Le(128)
			require.NotEqual(t, 0, connFd)

			// End of setup. Perform the test.

			write, err := tcp.Write([]byte("wazero"))
			require.NoError(t, err)
			require.NotEqual(t, 0, write)

			iovs := uint32(1)         // arbitrary offset
			resultNread := uint32(34) // arbitrary offset
			expectedMemory := append(tc.initialMemory, tc.expectedMemory...)
			maskMemory(t, mod, len(expectedMemory))

			ok := mod.Memory().Write(0, tc.initialMemory)
			require.True(t, ok)

			// Special case this test: let us add a bit of delay
			// to avoid EAGAIN.
			if tc.flags == wasip1.RECV_PEEK {
				time.Sleep(500 * time.Millisecond)
			}

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.SockRecvName, uint64(connFd), uint64(iovs), tc.iovsCount, uint64(tc.flags), uint64(resultNread), uint64(resultNread+8))
			require.Equal(t, fmt.Sprintf(tc.expectedLog, sys.FdPreopen, connFd, tc.flags), "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)
		})
	}
}

func Test_sockSend(t *testing.T) {
	tests := []struct {
		name           string
		funcName       string
		flags          uint32
		expectedErrno  wasip1.Errno
		expectedLog    string
		initialMemory  []byte
		iovsCount      uint64
		expectedMemory []byte
	}{
		{
			name:      "sock_send",
			iovsCount: 3,
			initialMemory: []byte{
				'?',         // `iovs` is after this
				18, 0, 0, 0, // = iovs[0].offset
				4, 0, 0, 0, // = iovs[0].length
				23, 0, 0, 0, // = iovs[1].offset
				2, 0, 0, 0, // = iovs[1].length
				'?',                // iovs[0].offset is after this
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?',      // iovs[1].offset is after this
				'r', 'o', // iovs[1].length bytes
				'?',
			},
			expectedMemory: []byte{
				6, 0, 0, 0, 0, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				'?',
			},

			expectedLog: `
==> wasi_snapshot_preview1.sock_accept(fd=%[1]d,flags=0,result.fd=128)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.sock_send(fd=%[2]d,si_data=1,si_data_count=2,si_flags=0,result.so_datalen=26)
<== errno=ESUCCESS
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := experimentalnet.WithConfig(testCtx, experimentalnet.NewConfig().WithTCPListener("127.0.0.1", 0))

			mod, r, log := requireProxyModuleWithContext(ctx, t, wazero.NewModuleConfig())
			defer r.Close(testCtx)

			// Dial the socket so that a call to accept doesn't hang.
			tcpAddr := requireTCPListenerAddr(t, mod)
			tcp, err := net.DialTCP("tcp", nil, tcpAddr)
			require.NoError(t, err)
			defer tcp.Close() //nolint

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.SockAcceptName, uint64(sys.FdPreopen), uint64(0), 128)
			connFd, _ := mod.Memory().ReadUint32Le(128)
			require.NotEqual(t, 0, connFd)

			// End of setup. Perform the test.
			iovs := uint32(1)            // arbitrary offset
			iovsCount := uint32(2)       // The count of iovs
			resultNwritten := uint32(26) // arbitrary offset
			expectedMemory := append(tc.initialMemory, tc.expectedMemory...)

			maskMemory(t, mod, len(expectedMemory))
			ok := mod.Memory().Write(0, tc.initialMemory)
			require.True(t, ok)

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.SockSendName, uint64(connFd), uint64(iovs), uint64(iovsCount), 0, uint64(resultNwritten))
			require.Equal(t, fmt.Sprintf(tc.expectedLog, sys.FdPreopen, connFd, tc.flags), "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
			require.True(t, ok)
			require.Equal(t, expectedMemory, actual)

			// Read back the value that was sent on the socket.
			buf := make([]byte, 10)
			read, err := tcp.Read(buf)
			require.NoError(t, err)
			require.NotEqual(t, 0, read)
			// Sometimes `buf` is smaller than len("wazero").
			require.True(t, strings.HasPrefix("wazero", string(buf[:read])))
		})
	}
}

type addr interface {
	Addr() *net.TCPAddr
}

func requireTCPListenerAddr(t *testing.T, mod api.Module) *net.TCPAddr {
	sock, ok := mod.(*wasm.ModuleInstance).Sys.FS().LookupFile(sys.FdPreopen)
	require.True(t, ok)
	return sock.File.(addr).Addr()
}
