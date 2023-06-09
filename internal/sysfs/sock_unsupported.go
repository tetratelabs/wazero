//go:build !linux && !darwin && !windows

package sysfs

import (
	"syscall"
)

// MSG_PEEK is a filler value
const MSG_PEEK = 0x2

func recvfromPeek(conn interface{}, p []byte) (n int, errno syscall.Errno) {
	return 0, syscall.ENOSYS
}
