package platform

import (
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWinSockFdSet(t *testing.T) {
	allSet := WinSockFdSet{
		count: _FD_SETSIZE,
	}
	for i := 0; i < _FD_SETSIZE; i++ {
		allSet.handles[i] = syscall.Handle(i)
	}
	shiftedFields := WinSockFdSet{
		count: _FD_SETSIZE - 1,
	}
	for i := 0; i < _FD_SETSIZE; i++ {
		shiftedFields.handles[i] = syscall.Handle(i)
	}
	for i := _FD_SETSIZE / 2; i < _FD_SETSIZE-1; i++ {
		shiftedFields.handles[i] = syscall.Handle(i + 1)
	}

	tests := []struct {
		name     string
		init     WinSockFdSet
		exec     func(fdSet *WinSockFdSet)
		expected WinSockFdSet
	}{
		{
			name: "all fields set",
			exec: func(fdSet *WinSockFdSet) {
				for fd := 0; fd < _FD_SETSIZE; fd++ {
					fdSet.Set(fd)
				}
			},
			expected: allSet,
		},
		{
			name: "clear should shift all fields by one position",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				fdSet.Clear(_FD_SETSIZE / 2)
			},
			expected: shiftedFields,
		},
		{
			name: "zero should clear all fields",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				fdSet.Zero()
			},
			expected: WinSockFdSet{},
		},
		{
			name: "is-set should return true for all fields",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				for i := 0; i < fdSet.Count(); i++ {
					require.True(t, fdSet.IsSet(i))
				}
			},
			expected: allSet,
		},
		{
			name: "is-set should return true for all odd bits",
			init: WinSockFdSet{},
			exec: func(fdSet *WinSockFdSet) {
				for fd := 1; fd < _FD_SETSIZE; fd += 2 {
					fdSet.Set(fd)
				}
				for fd := 0; fd < _FD_SETSIZE; fd++ {
					isSet := fdSet.IsSet(fd)
					if fd&0x1 == 0x1 {
						require.True(t, isSet)
					} else {
						require.False(t, isSet)
					}
				}
				fdSet.Zero()
			},
			expected: WinSockFdSet{},
		},
		{
			name: "should clear all even bits",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				for fd := 0; fd < _FD_SETSIZE; fd += 2 {
					fdSet.Clear(fd)
				}
				for fd := 0; fd < _FD_SETSIZE; fd++ {
					isSet := fdSet.IsSet(fd)
					if fd&0x1 == 0x1 {
						require.True(t, isSet)
					} else {
						require.False(t, isSet)
					}
				}
				fdSet.Zero()
			},
			expected: WinSockFdSet{},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			x := tc.init
			tc.exec(&x)
			require.Equal(t, tc.expected, x)
		})
	}
}

func TestFdSet(t *testing.T) {
	t.Run("A pipe should be set in FdSet.Pipe", func(t *testing.T) {
		r, _, _ := os.Pipe()
		defer r.Close()

		fdSet := FdSet{}
		fdSet.Set(int(r.Fd()))

		require.Equal(t, syscall.Handle(r.Fd()), fdSet.Pipes().Get(0))
	})

	t.Run("A regular file should be set in FdSet.Regular", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "test")
		require.NoError(t, err)
		defer f.Close()

		fdSet := FdSet{}
		fdSet.Set(int(f.Fd()))

		require.Equal(t, syscall.Handle(f.Fd()), fdSet.Regular().Get(0))
	})

	t.Run("A socket should be set in FdSet.Socket", func(t *testing.T) {
		listen, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listen.Close()

		conn, err := listen.(*net.TCPListener).SyscallConn()
		require.NoError(t, err)

		conn.Control(func(fd uintptr) {
			fdSet := FdSet{}
			fdSet.Set(int(fd))
			require.Equal(t, syscall.Handle(fd), fdSet.Sockets().Get(0))
		})
	})
}
