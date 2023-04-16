//go:build linux

package platform

import (
	"syscall"
	"time"
)

// syscall_select invokes select on Unix (unless Darwin), with the given timeout Duration.
func syscall_select(n int, r, w, e *FdSet, timeout time.Duration) (int, error) {
	t := syscall.NsecToTimeval(timeout.Nanoseconds())
	return syscall.Select(n, (*syscall.FdSet)(r), (*syscall.FdSet)(w), (*syscall.FdSet)(e), &t)
}
