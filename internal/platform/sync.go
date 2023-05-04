//go:build !windows

package platform

import (
	"io/fs"
	"syscall"
)

func sync(f fs.File) syscall.Errno {
	if s, ok := f.(syncFile); ok {
		return UnwrapOSError(s.Sync())
	}
	return 0
}
