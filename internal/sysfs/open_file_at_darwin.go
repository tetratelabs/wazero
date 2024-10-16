//go:build !tinygo

package sysfs

import (
	"io/fs"
	"os"
	"path"
	"syscall"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func openFileAt(dir *os.File, filePath string, oflag sys.Oflag, perm fs.FileMode) (*os.File, sys.Errno) {
	fd, err := openat(int(dir.Fd()), filePath, toOsOpenFlag(oflag), uint32(perm))
	if err != nil {
		return nil, sys.UnwrapOSError(err)
	}

	return os.NewFile(uintptr(fd), path.Base(filePath)), 0
}

func openat(dirfd int, path string, mode int, perm uint32) (fd int, err error) {
	var p0 *byte

	p0, err = syscall.BytePtrFromString(path)
	if err != nil {
		return
	}

	r0, _, e1 := syscall_syscall6(libc_openat_trampoline_addr, uintptr(dirfd), uintptr(unsafe.Pointer(p0)), uintptr(mode), uintptr(perm), 0, 0)
	fd = int(r0)
	if e1 != 0 {
		err = sys.UnwrapOSError(e1)
	}

	return
}

//go:cgo_import_dynamic libc_openat openat "/usr/lib/libSystem.B.dylib"

var libc_openat_trampoline_addr uintptr
