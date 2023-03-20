package platform

import (
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_mapToWindowsHandle(t *testing.T) {
	require.Equal(t, uintptr(syscall.Stdin), mapToWindowsHandle(0))
	require.Equal(t, uintptr(syscall.Stdout), mapToWindowsHandle(1))
	require.Equal(t, uintptr(syscall.Stderr), mapToWindowsHandle(2))
}
