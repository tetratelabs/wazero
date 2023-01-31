//go:build !windows

package sysfs

import "syscall"

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

func maybeWrapFile(f file) file {
	return f
}
