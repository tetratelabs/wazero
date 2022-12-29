package writefs

import (
	"io/fs"
	"syscall"
)

const (
	// ERROR_ACCESS_DENIED is a Windows error returned by syscall.Unlink
	// instead of syscall.EPERM
	ERROR_ACCESS_DENIED = syscall.Errno(5)

	// ERROR_ALREADY_EXISTS is a Windows error returned by os.Mkdir
	// instead of syscall.EEXIST
	ERROR_ALREADY_EXISTS = syscall.Errno(183)

	// ERROR_DIRECTORY is a Windows error returned by syscall.Rmdir
	// instead of syscall.ENOTDIR
	ERROR_DIRECTORY = syscall.Errno(267)
)

func adjustMkdirError(err error) error {
	// os.Mkdir wraps the syscall error in a path error
	if pe, ok := err.(*fs.PathError); ok && pe.Err == ERROR_ALREADY_EXISTS {
		pe.Err = syscall.EEXIST // adjust it
	}
	return err
}

func adjustRmdirError(err error) error {
	if err == ERROR_DIRECTORY {
		return syscall.ENOTDIR
	}
	return err
}

func adjustUnlinkError(err error) error {
	if err == ERROR_ACCESS_DENIED {
		return syscall.EISDIR
	}
	return err
}
