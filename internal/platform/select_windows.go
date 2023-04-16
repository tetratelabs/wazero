package platform

import (
	"syscall"
	"time"
)

const WasiFdStdin = 0

// syscall_select emulates the select syscall on Windows for two, well-known cases, returns syscall.ENOSYS for all others.
// If r contains fd 0, then it immediately returns 1 (data ready on stdin) and r will have the fd 0 bit set.
// If n==0 it will wait for the given timeout duration.
func syscall_select(n int, r, w, e *FdSet, timeout time.Duration) (int, error) {
	if n == 0 {
		time.Sleep(timeout)
		return 0, nil
	}
	if r.IsSet(WasiFdStdin) {
		r.Zero()
		r.Set(WasiFdStdin)
		return 1, nil
	}
	return -1, syscall.ENOSYS
}
