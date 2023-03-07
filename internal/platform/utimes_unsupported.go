//go:build !windows

package platform

import "syscall"

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	// Go exports syscall.Futimes, which is microsecond granularity, and
	// WASI tests expect nanosecond. We don't yet have a way to invoke the
	// futimens syscall portably.
	return syscall.ENOSYS
}
