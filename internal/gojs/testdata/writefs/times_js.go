package writefs

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64) {
	d := t.Sys().(*syscall.Stat_t)
	return d.Atime*1e9 + d.AtimeNsec, d.Mtime*1e9 + d.MtimeNsec, d.Ctime*1e9 + d.CtimeNsec
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	d := t.Sys().(*syscall.Stat_t)
	return uint64(d.Dev), uint64(d.Ino)
}
