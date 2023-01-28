package writefs

import (
	"os"
	"syscall"
)

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	d := t.Sys().(*syscall.Stat_t)
	return d.Atime*1e9 + d.AtimeNsec, d.Mtime*1e9 + d.MtimeNsec, d.Ctime*1e9 + d.CtimeNsec, uint64(d.Nlink)
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	d := t.Sys().(*syscall.Stat_t)
	return uint64(d.Dev), uint64(d.Ino)
}
