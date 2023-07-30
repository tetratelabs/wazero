//go:build !linux && !darwin && !windows

package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
)

// poll implements `Poll` as documented on sys.File via a file descriptor.
func poll(fd uintptr, flag sys.Pflag, timeoutMillis int32) (ready bool, errno sys.Errno) {
	return false, sys.ENOSYS
}
