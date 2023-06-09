//go:build windows

package sysfs

import "syscall"

func setNonblock(fd Sysfd, enable bool) error {
	return syscall.SetNonblock(fd, enable)
}
