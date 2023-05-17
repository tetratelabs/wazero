package fstest

import (
	"io/fs"
	"syscall"
)

func timesFromFileInfo(t fs.FileInfo) (atim, mtime int64) {
	if d, ok := t.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.LastAccessTime.Nanoseconds(), d.LastWriteTime.Nanoseconds()
	} else {
		panic("unexpected")
	}
}
