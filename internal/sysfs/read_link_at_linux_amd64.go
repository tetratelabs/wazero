//go:build !tinygo

package sysfs

import (
	"os"
	"syscall"
	"unsafe"
)

// ReadLinkAt first calls `fstatat(2)` on the file `path` resolves to get the
// size of the symlink.  It then calls `readlinkat(2)` to get the actual link.
// A race condition is possible if the link size changed between the two
// calls, in which case we return `syscall.EOVERFLOW`.
func ReadLinkAt(dir *os.File, path string) (string, error) {
	pathPtr, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", err
	}

	var stat syscall.Stat_t

	_, _, errno := syscall.Syscall6(
		syscall.SYS_NEWFSTATAT,
		dir.Fd(),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&stat)),
		uintptr(AT_SYMLINK_NOFOLLOW),
		0,
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

	r, _, errno := syscall.Syscall6(
		syscall.SYS_READLINKAT,
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
