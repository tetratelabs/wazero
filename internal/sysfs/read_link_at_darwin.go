package sysfs

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// ReadLinkAt first calls `lstat(2)` on the file to get the size of the symlink.
// It then calls `readlinkat(2)` to get the actual link.  This implementation
// differs from the Linux one because `fstatat(2)` does not support
// `AT_SYMLINK_NOFOLLOW` on Darwin.
func ReadLinkAt(dir *os.File, path string) (string, error) {
	fullPath := filepath.Join(dir.Name(), path)

	pathPtr, err := syscall.BytePtrFromString(fullPath)
	if err != nil {
		return "", err
	}

	var stat syscall.Stat_t

	_, _, errno := syscall_syscall(
		libc_lstat64_trampoline_addr,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&stat)),
		0,
	)
	if errno != 0 {
		return "", errno
	}

	buf := make([]byte, stat.Size)
	bufPtr := unsafe.Pointer(&zero)

	if len(buf) > 0 {
		bufPtr = unsafe.Pointer(&buf[0])
	}

	r, _, errno := syscall_syscall6(
		libc_readlinkat_trampoline_addr,
		dir.Fd(),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(bufPtr)),
		uintptr(len(buf)),
		0,
		0,
	)
	if errno != 0 {
		return "", errno
	}

	if int64(r) != stat.Size {
		return "", syscall.EOVERFLOW
	}

	return string(buf), nil
}

//go:cgo_import_dynamic libc_lstat64 lstat64 "/usr/lib/libSystem.B.dylib"
var libc_lstat64_trampoline_addr uintptr

//go:cgo_import_dynamic libc_readlinkat readlinkat "/usr/lib/libSystem.B.dylib"
var libc_readlinkat_trampoline_addr uintptr
