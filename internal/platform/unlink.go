//go:build !windows

package platform

import "syscall"

func Unlink(name string) (err error) {
	err = syscall.Unlink(name)
	return
}
