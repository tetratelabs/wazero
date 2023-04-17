package platform

import "time"

func Select(n int, r, w, e *FdSet, timeout *time.Duration) (int, error) {
	return syscall_select(n, r, w, e, timeout)
}
