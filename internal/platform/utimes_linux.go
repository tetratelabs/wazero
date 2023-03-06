//go:build !darwin

package platform

import (
	"syscall"
	"unsafe"
)

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	times := []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	}
	_, _, err := syscall.Syscall6(syscall.SYS_UTIMENSAT, fd, uintptr(0), uintptr(unsafe.Pointer(&times[0])), uintptr(0), 0, 0)
	if err != 0 {
		return err
	}
	return nil
}
