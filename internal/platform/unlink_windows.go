package platform

import (
	"syscall"
	"unsafe"
)

var procDeleteFileW = kernel32.NewProc("DeleteFileW")

func Unlink(name string) (err error) {
	pathp, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}

	r1, _, e1 := syscall.Syscall(procDeleteFileW.Addr(), 2, uintptr(unsafe.Pointer(pathp)),
		1 /* force delete */, 0)
	if r1 == 0 {
		err = e1
	}
	return
}
