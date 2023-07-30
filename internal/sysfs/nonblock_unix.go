//go:build !windows && !plan9

package sysfs

import (
	"syscall"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
)

func setNonblock(fd uintptr, enable bool) sys.Errno {
	return sys.UnwrapOSError(syscall.SetNonblock(int(fd), enable))
}

func isNonblock(f *osFile) bool {
	return f.flag&fsapi.O_NONBLOCK == fsapi.O_NONBLOCK
}
