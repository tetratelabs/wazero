package platform

import "time"

// Select exposes the select(2) syscall.
//
// For convenience, we expose a pointer to a time.Duration instead of a pointer to a syscall.Timeval.
// It must be a pointer because `nil` means "wait forever".
//
// However, notice that select(2) may mutate the pointed timeval on some platforms,
// for instance if the call returns early.
//
// This implementation *will not* update the pointed time.Duration value accordingly.
//
// See also: https://github.com/golang/sys/blob/master/unix/syscall_unix_test.go#L606-L617
func Select(n int, r, w, e *FdSet, timeout *time.Duration) (int, error) {
	return syscall_select(n, r, w, e, timeout)
}
