//go:build linux

package platform

import (
	"syscall"
	"time"
)

// syscall_select invokes select on Unix (unless Darwin), with the given timeout Duration.
func syscall_select(n int, r, w, e *FdSet, timeout *time.Duration) (int, error) {
	var t *syscall.Timeval
	if timeout != nil {
		tv := syscall.NsecToTimeval(timeout.Nanoseconds())
		t = &tv
	}
	return syscall.Select(n, (*syscall.FdSet)(r), (*syscall.FdSet)(w), (*syscall.FdSet)(e), t)
}
