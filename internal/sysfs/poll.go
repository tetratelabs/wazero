//go:build windows || linux || darwin

package sysfs

import "github.com/tetratelabs/wazero/experimental/sys"

// pollRead implements `PollRead` as documented on fsapi.File via a file
// descriptor.
func pollRead(fd uintptr, timeoutMillis int32) (ready bool, errno sys.Errno) {
	fds := []pollFd{newPollFd(fd, _POLLIN, 0)}
	count, errno := poll(fds, timeoutMillis)
	return count > 0, errno
}
