//go:build (amd64 || arm64) && windows

package platform

import (
	"os"
	"syscall"
)

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	d := t.Sys().(*syscall.Win32FileAttributeData)
	atimeNsec = d.LastAccessTime.Nanoseconds()
	mtimeNsec = d.LastWriteTime.Nanoseconds()
	ctimeNsec = d.CreationTime.Nanoseconds()
	// Even though we could get nlink from GetFileInformationByHandle API, the result is always one.
	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/ns-fileapi-by_handle_file_information
	nlink = 1
	return
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	// TODO: VolumeSerialNumber, FileIndexHigh and FileIndexLow are used in
	// os.SameFile, but the fields aren't exported or accessible in os.FileInfo
	// When we make our file type, get these from GetFileInformationByHandle.
	// Note that this requires access to the underlying FD number.
	return 0, 0
}
