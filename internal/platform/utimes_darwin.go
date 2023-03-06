package platform

import (
	"syscall"
	"unsafe"
)

var libc_futimens_trampoline_addr uintptr

//go:cgo_import_dynamic libc_futimens futimens "/usr/lib/libSystem.B.dylib"

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	tv := []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	}

	_, _, e1 := syscall_syscall6(libc_futimens_trampoline_addr, fd, uintptr(unsafe.Pointer(&tv[0])), 0, 0, 0, 0)
	if e1 != 0 {
		return e1
	}
	return nil
}

// we need to use this instead of syscall.Syscall6
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:linkname syscall_syscall6 syscall.syscall6
