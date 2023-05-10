//go:build !windows

package platform

import "syscall"

func setNonblock(fd uintptr, enable bool) error {
	return syscall.SetNonblock(int(fd), enable)
}
