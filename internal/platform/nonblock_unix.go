package platform

import "syscall"

func SetNonblock(fd uintptr, enable bool) error {
	return syscall.SetNonblock(int(fd), enable)
}
