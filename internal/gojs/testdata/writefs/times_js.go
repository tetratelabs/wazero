package writefs

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeSec, atimeNsec, mtimeSec, mtimeNsec, ctimeSec, ctimeNsec int64) {
	d := t.Sys().(*syscall.Stat_t)
	return d.Atime, d.AtimeNsec, d.Mtime, d.MtimeNsec, d.Ctime, d.CtimeNsec
}
