package platform

import (
	"io/fs"
	"os"
	"syscall"
	"unsafe"
)

func OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	fd, err := open(name, flag|syscall.O_CLOEXEC, uint32(perm))
	if err == nil {
		return os.NewFile(uintptr(fd), name), nil
	}
	// TODO: Set FILE_SHARE_DELETE for directory as well.
	return os.OpenFile(name, flag, perm)
}

// The following is lifted from syscall_windows.go to add support for setting FILE_SHARE_DELETE.
// https://github.com/golang/go/blob/go1.19.5/src/syscall/syscall_windows.go#L307-L375
func open(path string, mode int, perm uint32) (fd syscall.Handle, err error) {
	if len(path) == 0 {
		return syscall.InvalidHandle, syscall.ERROR_FILE_NOT_FOUND
	}
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	var access uint32
	switch mode & (syscall.O_RDONLY | syscall.O_WRONLY | syscall.O_RDWR) {
	case syscall.O_RDONLY:
		access = syscall.GENERIC_READ
	case syscall.O_WRONLY:
		access = syscall.GENERIC_WRITE
	case syscall.O_RDWR:
		access = syscall.GENERIC_READ | syscall.GENERIC_WRITE
	}
	if mode&syscall.O_CREAT != 0 {
		access |= syscall.GENERIC_WRITE
	}
	if mode&syscall.O_APPEND != 0 {
		access &^= syscall.GENERIC_WRITE
		access |= syscall.FILE_APPEND_DATA
	}
	sharemode := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE)
	var sa *syscall.SecurityAttributes
	if mode&syscall.O_CLOEXEC == 0 {
		var _sa syscall.SecurityAttributes
		_sa.Length = uint32(unsafe.Sizeof(sa))
		_sa.InheritHandle = 1
		sa = &_sa
	}
	var createmode uint32
	switch {
	case mode&(syscall.O_CREAT|syscall.O_EXCL) == (syscall.O_CREAT | syscall.O_EXCL):
		createmode = syscall.CREATE_NEW
	case mode&(syscall.O_CREAT|syscall.O_TRUNC) == (syscall.O_CREAT | syscall.O_TRUNC):
		createmode = syscall.CREATE_ALWAYS
	case mode&syscall.O_CREAT == syscall.O_CREAT:
		createmode = syscall.OPEN_ALWAYS
	case mode&syscall.O_TRUNC == syscall.O_TRUNC:
		createmode = syscall.TRUNCATE_EXISTING
	default:
		createmode = syscall.OPEN_EXISTING
	}
	var attrs uint32 = syscall.FILE_ATTRIBUTE_NORMAL
	if perm&syscall.S_IWRITE == 0 {
		attrs = syscall.FILE_ATTRIBUTE_READONLY
		if createmode == syscall.CREATE_ALWAYS {
			// We have been asked to create a read-only file.
			// If the file already exists, the semantics of
			// the Unix open system call is to preserve the
			// existing permissions. If we pass CREATE_ALWAYS
			// and FILE_ATTRIBUTE_READONLY to CreateFile,
			// and the file already exists, CreateFile will
			// change the file permissions.
			// Avoid that to preserve the Unix semantics.
			h, e := syscall.CreateFile(pathp, access, sharemode, sa, syscall.TRUNCATE_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
			switch e {
			case syscall.ERROR_FILE_NOT_FOUND, syscall.ERROR_PATH_NOT_FOUND:
				// File does not exist. These are the same
				// errors as Errno.Is checks for ErrNotExist.
				// Carry on to create the file.
			default:
				// Success or some different error.
				return h, e
			}
		}
	}
	h, e := syscall.CreateFile(pathp, access, sharemode, sa, createmode, attrs, 0)
	return h, e
}
