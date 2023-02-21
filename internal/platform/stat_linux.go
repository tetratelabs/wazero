//go:build (amd64 || arm64 || riscv64) && linux

// Note: This expression is not the same as compiler support, even if it looks
// similar. Platform functions here are used in interpreter mode as well.

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
	stat.Ino = uint64(d.Ino)
	stat.Dev = uint64(d.Dev)
	stat.Mode = t.Mode()
	stat.Nlink = uint64(d.Nlink)
	stat.Size = d.Size
	atime := d.Atim
	stat.Atim = atime.Sec*1e9 + atime.Nsec
	mtime := d.Mtim
	stat.Mtim = mtime.Sec*1e9 + mtime.Nsec
	ctime := d.Ctim
	stat.Ctim = ctime.Sec*1e9 + ctime.Nsec
}
