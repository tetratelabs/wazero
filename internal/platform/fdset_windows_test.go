package platform

import (
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
			name: "all bits cleared",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				for fd := 0; fd < _FD_SETSIZE; fd++ {
					fdSet.Clear(fd)
				}
			},
			expected: WinSockFdSet{},
		},
		{
			name: "zero should clear all bits",
			init: allSet,
			exec: func(fdSet *WinSockFdSet) {
				fdSet.Zero()
			},
			expected: WinSockFdSet{},
		},
		{
			name: "is-set should return true for all bits",
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
