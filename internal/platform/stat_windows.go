package platform

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeSec, atimeNsec, mtimeSec, mtimeNsec, ctimeSec, ctimeNsec int64) {
	d := t.Sys().(*syscall.Win32FileAttributeData)
	atime := d.LastAccessTime.Nanoseconds()
	mtime := d.LastWriteTime.Nanoseconds()
	ctime := d.CreationTime.Nanoseconds()
	return atime / 1e9, atime % 1e9, mtime / 1e9, mtime % 1e9, ctime / 1e9, ctime % 1e9
}
