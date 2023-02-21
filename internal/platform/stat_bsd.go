//go:build (amd64 || arm64) && (darwin || freebsd)

package platform

import (
	"os"
	"syscall"
)

func stat(path string, st *Stat_t) (err error) {
	t, err := os.Stat(path)
	if err = UnwrapOSError(err); err == nil {
		fillStatFromSys(st, t)
	}
	return
}

func fillStatFromOpenFile(stat *Stat_t, fd uintptr, t os.FileInfo) (err error) {
	fillStatFromSys(stat, t)
	return
}

func fillStatFromSys(stat *Stat_t, t os.FileInfo) {
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
}
