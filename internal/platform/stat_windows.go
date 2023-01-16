//go:build (amd64 || arm64) && windows

package platform

import (
	"os"
	"syscall"
)

func statTimes(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64) {
	d := t.Sys().(*syscall.Win32FileAttributeData)
	atimeNsec = d.LastAccessTime.Nanoseconds()
	mtimeNsec = d.LastWriteTime.Nanoseconds()
	ctimeNsec = d.CreationTime.Nanoseconds()
	return
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	// TODO: VolumeSerialNumber, FileIndexHigh and FileIndexLow are used in
	// os.SameFile, but the fields aren't exported or accessible in os.FileInfo
	// When we make our file type, get these from GetFileInformationByHandle.
	// Note that this requires access to the underlying FD number.
	return 0, 0
}
