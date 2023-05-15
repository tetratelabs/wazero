//go:build !windows

package platform

import (
	"os"
	"syscall"
)

func sync(f *os.File) syscall.Errno {
	return UnwrapOSError(f.Sync())
}
