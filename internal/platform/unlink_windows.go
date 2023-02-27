//go:build windows

package platform

import (
	"os"
	"syscall"
)

func Unlink(name string) (err error) {
	err = syscall.Unlink(name)
	if err == nil {
		return
	}
	err = UnwrapOSError(err)
	if err == syscall.EPERM {
		_, errLstat := os.Lstat(name)
		if errLstat == nil {
			err = UnwrapOSError(os.Remove(name))
		}
	}
	return
}
