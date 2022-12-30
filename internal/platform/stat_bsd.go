//go:build darwin || freebsd

package platform

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeSec, atimeNSec, mtimeSec, mtimeNSec, ctimeSec, ctimeNSec int64) {
	d := t.Sys().(*syscall.Stat_t)
	atime := d.Atimespec
	mtime := d.Mtimespec
	ctime := d.Ctimespec
	return atime.Sec, atime.Nsec, mtime.Sec, mtime.Nsec, ctime.Sec, ctime.Nsec
}
