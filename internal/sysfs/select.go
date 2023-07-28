package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
)

// _select waits until one or more of the file descriptors become ready for
// reading or writing.
//
// # Parameters
//
// The `timeoutMillis` parameter is how long to block for an event, or
// interrupted, in milliseconds. There are two special values:
//   - zero returns immediately
//   - any negative value blocks any amount of time
//
// A zero sys.Errno is success. The below are expected otherwise:
//   - sys.ENOSYS: the implementation does not support this function.
//   - sys.EINTR: the call was interrupted prior to an event.
//
// # Impact of blocking
//
//	Because this is a blocking syscall, it will also block the carrier thread of the goroutine,
//	preventing any means to support context cancellation directly.
//
//	There are ways to obviate this issue. We outline here one idea, that is however not currently implemented.
//	A common approach to support context cancellation is to add a signal file descriptor to the set,
//	e.g. the read-end of a pipe or an eventfd on Linux.
//	When the context is canceled, we may unblock a Select call by writing to the fd, causing it to return immediately.
//	This however requires to do a bit of housekeeping to hide the "special" FD from the end-user.
//
// # Notes
//
//   - This is like `select` in POSIX except it returns if any are ready
//     instead of a specific file descriptor. See
//     https://pubs.opengroup.org/onlinepubs/9699919799/functions/select.html
//   - This is named _select to avoid collision on the select keyword, while
//     not exporting the function.
func _select(n int, r, w, e *platform.FdSet, timeoutNanos int32) (ready bool, errno sys.Errno) {
	return syscall_select(n, r, w, e, timeoutNanos)
}
