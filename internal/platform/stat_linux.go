package platform

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeSec, atimeNSec, mtimeSec, mtimeNSec, ctimeSec, ctimeNSec int64) {
	d := t.Sys().(*syscall.Stat_t)
	atime := d.Atim
	mtime := d.Mtim
	ctime := d.Ctim
	return atime.Sec, atime.Nsec, mtime.Sec, mtime.Nsec, ctime.Sec, ctime.Nsec
}
