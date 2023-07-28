//go:build !linux && !darwin && !windows

package sysfs

import "github.com/tetratelabs/wazero/experimental/sys"

// pollRead implements `PollRead` as documented on fsapi.File via a file
// descriptor.
func pollRead(fd uintptr, timeoutMillis int32) (ready bool, errno sys.Errno) {
	return false, sys.ENOSYS
}
