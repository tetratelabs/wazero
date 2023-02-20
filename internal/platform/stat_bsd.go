//go:build (amd64 || arm64) && (darwin || freebsd)

package platform

import (
	"os"
	"syscall"
)

func fillStatFromOpenFile(stat *Stat_t, fd uintptr, t os.FileInfo) (err error) {
	d := t.Sys().(*syscall.Stat_t)
	stat.Ino = d.Ino
	stat.Dev = uint64(d.Dev)
	stat.Mode = t.Mode()
	stat.Nlink = uint64(d.Nlink)
	stat.Size = d.Size
	atime := d.Atimespec
	stat.Atim = atime.Sec*1e9 + atime.Nsec
	mtime := d.Mtimespec
	stat.Mtim = mtime.Sec*1e9 + mtime.Nsec
	ctime := d.Ctimespec
	stat.Ctim = ctime.Sec*1e9 + ctime.Nsec
	return
}
