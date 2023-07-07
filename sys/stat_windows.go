//go:build (amd64 || arm64) && windows

package sys

import (
	"io/fs"
	"syscall"
)

const sysParseable = true

func statFromFileInfo(t fs.FileInfo) Stat_t {
	if d, ok := t.Sys().(*syscall.Win32FileAttributeData); ok {
		st := Stat_t{}
		st.Ino = 0 // not in Win32FileAttributeData
		st.Dev = 0 // not in Win32FileAttributeData
		st.Mode = t.Mode()
		st.Nlink = 1 // not in Win32FileAttributeData
		st.Size = t.Size()
		st.Atim = d.LastAccessTime.Nanoseconds()
		st.Mtim = d.LastWriteTime.Nanoseconds()
		st.Ctim = d.CreationTime.Nanoseconds()
		return st
	}
	return defaultStatFromFileInfo(t)
}
