//go:build (amd64 || arm64) && (darwin || freebsd)

package platform

import (
	"os"
	"syscall"
)

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	d := t.Sys().(*syscall.Stat_t)
	atime := d.Atimespec
	mtime := d.Mtimespec
	ctime := d.Ctimespec
	return atime.Sec*1e9 + atime.Nsec, mtime.Sec*1e9 + mtime.Nsec, ctime.Sec*1e9 + ctime.Nsec, uint64(d.Nlink)
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	d := t.Sys().(*syscall.Stat_t)
	dev = uint64(d.Dev)
	inode = d.Ino
	return
}
