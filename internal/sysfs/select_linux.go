package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
)

// syscall_select implements _select on Linux
func syscall_select(n int, r, w, e *platform.FdSet, timeoutNanos int32) (bool, sys.Errno) {
	var t *syscall.Timeval
	if timeoutNanos >= 0 {
		tv := syscall.NsecToTimeval(int64(timeoutNanos))
		t = &tv
	}
	n, err := syscall.Select(n, (*syscall.FdSet)(r), (*syscall.FdSet)(w), (*syscall.FdSet)(e), t)
	return n > 0, sys.UnwrapOSError(err)
}
