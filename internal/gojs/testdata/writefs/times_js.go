package writefs

import (
	"io/fs"
	"os"
	"syscall"
)

func Stat(_ fs.File, t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink, dev, inode uint64, err error) {
	d := t.Sys().(*syscall.Stat_t)
	return d.Atime*1e9 + d.AtimeNsec, d.Mtime*1e9 + d.MtimeNsec, d.Ctime*1e9 + d.CtimeNsec, 0, uint64(d.Dev), uint64(d.Ino), nil
}
