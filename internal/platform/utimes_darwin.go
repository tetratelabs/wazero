package platform

import (
	"syscall"
	"unsafe"
)

func futimens(fd uintptr, atimeNsec, mtimeNsec int64) error {
	tv := []syscall.Timespec{
		syscall.NsecToTimespec(atimeNsec),
		syscall.NsecToTimespec(mtimeNsec),
	}

	// Warning: futimens only exists since High Sierra (10.13).
	_, _, e1 := syscall_syscall6(libc_futimens_trampoline_addr, fd, uintptr(unsafe.Pointer(&tv[0])), 0, 0, 0, 0)
	if e1 != 0 {
		return e1
	}
	return nil
}

// libc_futimens_trampoline_addr is the address of the
// `libc_futimens_trampoline` symbol, defined in `utimes_darwin.s`.
//
// We use this to invoke the syscall through syscall_syscall6 imported below.
var libc_futimens_trampoline_addr uintptr

// Imports the futimens symbol from libc as `libc_futimens`.
//
// Note: CGO mechanisms are used in darwin regardless of the CGO_ENABLED value
// or the "cgo" build flag. See /RATIONALE.md for why.
//go:cgo_import_dynamic libc_futimens futimens "/usr/lib/libSystem.B.dylib"

// syscall_syscall6 is a private symbol that we link below. We need to use this
// instead of syscall.Syscall6 because the public syscall.Syscall6 won't work
// when fn is an address.
//
//go:linkname syscall_syscall6 syscall.syscall6
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)
