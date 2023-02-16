package writefs

import (
	"fmt"
	"os"
	"syscall"
)

func statFields(path string) (atimeNsec, mtimeNsec int64, dev, inode uint64) {
	if t, err := os.Stat(path); err != nil {
		panic(fmt.Errorf("failed to stat path %s: %v", path, err))
	} else {
		d := t.Sys().(*syscall.Stat_t)
		return d.Atime*1e9 + d.AtimeNsec, d.Mtime*1e9 + d.MtimeNsec, uint64(d.Dev), uint64(d.Ino)
	}
}
