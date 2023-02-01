//go:build !windows

package sysfs

import (
	"io/fs"
	"syscall"
)

func adjustMkdirError(err error) error {
	return err
}

func adjustRmdirError(err error) error {
	return err
}

func adjustTruncateError(err error) error {
	return err
}

func adjustUnlinkError(err error) error {
	if err == syscall.EPERM {
		return syscall.EISDIR
	}
	return err
}

func maybeWrapFile(f file, _ FS, _ string, _ int, _ fs.FileMode) file {
	return f
}
