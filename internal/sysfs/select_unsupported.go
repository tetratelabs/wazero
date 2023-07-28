//go:build !darwin && !linux && !windows

package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
)

func syscall_select(n int, r, w, e *platform.FdSet, timeoutNanos int32) (ready bool, errno sys.Errno) {
	return false, sys.ENOSYS
}
