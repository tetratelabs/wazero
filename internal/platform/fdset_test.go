package platform

import (
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFdSet(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("not supported")
	}

	tests := []struct {
		name     string
		init     FdSet
		exec     func(fdSet *FdSet)
		expected FdSet
	}{
		{
			name: "all bits set",
			exec: func(fdSet *FdSet) {
				for fd := 0; fd < nfdbits; fd++ {
					fdSet.Set(fd)
				}
			},
			expected: FdSet{Bits: fill(-1)},
		},
		{
			name: "all bits cleared",
			init: FdSet{Bits: fill(-1)},
			exec: func(fdSet *FdSet) {
				for fd := 0; fd < nfdbits; fd++ {
					fdSet.Clear(fd)
				}
			},
			expected: FdSet{},
		},
		{
			name: "zero should clear all bits",
			init: FdSet{Bits: fill(-1)},
			exec: func(fdSet *FdSet) {
				fdSet.Zero()
			},
			expected: FdSet{},
		},
		{
			name: "is-set should return true for all bits",
			init: FdSet{Bits: fill(-1)},
			exec: func(fdSet *FdSet) {
				for i := range fdSet.Bits {
					require.True(t, fdSet.IsSet(i))
				}
			},
			expected: FdSet{Bits: fill(-1)},
		},
		{
			name: "is-set should return true for all odd bits",
			init: FdSet{},
			exec: func(fdSet *FdSet) {
				for fd := 1; fd < nfdbits; fd += 2 {
					fdSet.Set(fd)
				}
				for fd := 0; fd < nfdbits; fd++ {
					isSet := fdSet.IsSet(fd)
					if fd&0x1 == 0x1 {
						require.True(t, isSet)
					} else {
						require.False(t, isSet)
					}
				}
				fdSet.Zero()
			},
			expected: FdSet{},
		},
		{
			name: "should clear all even bits",
			init: FdSet{Bits: fill(-1)},
			exec: func(fdSet *FdSet) {
				for fd := 0; fd < nfdbits; fd += 2 {
					fdSet.Clear(fd)
				}
				for fd := 0; fd < nfdbits; fd++ {
					isSet := fdSet.IsSet(fd)
					if fd&0x1 == 0x1 {
						require.True(t, isSet)
					} else {
						require.False(t, isSet)
					}
				}
				fdSet.Zero()
			},
			expected: FdSet{},
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
