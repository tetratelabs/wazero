//go:build windows

package platform

import "syscall"

func SetNonblock(fd uintptr, enable bool) error {
	return syscall.SetNonblock(syscall.Handle(fd), enable)
}
