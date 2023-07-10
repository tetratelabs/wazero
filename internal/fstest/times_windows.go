package fstest

import (
	"io/fs"
	"syscall"
)

func timesFromFileInfo(info fs.FileInfo) (atim, mtime int64) {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.LastAccessTime.Nanoseconds(), d.LastWriteTime.Nanoseconds()
	} else {
		panic("unexpected")
	}
}
