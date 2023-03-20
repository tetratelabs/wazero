//go:build (amd64 || arm64) && (darwin || freebsd)

package platform

import (
	"io/fs"
	"os"
	"syscall"
)

func lstat(path string) (Stat_t, error) {
	if t, err := os.Lstat(path); err != nil {
		return Stat_t{}, err
	} else {
		return statFromFileInfo(t), nil
	}
}

func stat(path string) (Stat_t, error) {
	if t, err := os.Stat(path); err != nil {
		return Stat_t{}, err
	} else {
		return statFromFileInfo(t), nil
	}
}

func statFile(f fs.File) (Stat_t, error) {
	return defaultStatFile(f)
}

func inoFromFileInfo(_ readdirFile, t fs.FileInfo) (ino uint64, err error) {
	if d, ok := t.Sys().(*syscall.Stat_t); ok {
		ino = d.Ino
	}
	return
}

func statFromFileInfo(t fs.FileInfo) Stat_t {
	if d, ok := t.Sys().(*syscall.Stat_t); ok {
		st := Stat_t{}
		st.Dev = uint64(d.Dev)
		st.Ino = d.Ino
		st.Uid = d.Uid
		st.Gid = d.Gid
		st.Mode = t.Mode()
		st.Nlink = uint64(d.Nlink)
		st.Size = d.Size
		atime := d.Atimespec
		st.Atim = atime.Sec*1e9 + atime.Nsec
		mtime := d.Mtimespec
		st.Mtim = mtime.Sec*1e9 + mtime.Nsec
		ctime := d.Ctimespec
		st.Ctim = ctime.Sec*1e9 + ctime.Nsec
		return st
	}
	return statFromDefaultFileInfo(t)
}
