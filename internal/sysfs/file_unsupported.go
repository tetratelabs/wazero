//go:build !unix && !linux && !darwin && !windows

package sysfs

import "syscall"

const nonBlockingFileIoSupported = false

// readFd returns ENOSYS on unsupported platforms.
func readFd(fd uintptr, buf []byte) (int, syscall.Errno) {
	return -1, syscall.ENOSYS
}
