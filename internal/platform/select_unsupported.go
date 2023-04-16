//go:build !unix && !linux && !windows

package platform

import (
	"syscall"
	"time"
)

func syscall_select(n int, r, w, e *FdSet, timeout time.Duration) (int, error) {
	return -1, syscall.ENOSYS
}
