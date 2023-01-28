//go:build (amd64 || arm64 || riscv64) && linux

// Note: This expression is not the same as compiler support, even if it looks
// similar. Platform functions here are used in interpreter mode as well.

package platform

import (
	"os"
	"syscall"
)

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	d := t.Sys().(*syscall.Stat_t)
	atime := d.Atim
	mtime := d.Mtim
	ctime := d.Ctim
	return atime.Sec*1e9 + atime.Nsec, mtime.Sec*1e9 + mtime.Nsec, ctime.Sec*1e9 + ctime.Nsec, uint64(d.Nlink)
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	d := t.Sys().(*syscall.Stat_t)
	dev = d.Dev
	inode = d.Ino
	return
}
