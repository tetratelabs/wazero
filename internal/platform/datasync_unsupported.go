//go:build !linux

package platform

import (
	"io/fs"
	"syscall"
)

func datasync(f fs.File) syscall.Errno {
	// Attempt to sync everything, even if we only need to sync the data.
	return sync(f)
}
