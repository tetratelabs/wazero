//go:build (amd64 || arm64) && windows

package platform

import (
	"io/fs"
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

type fdGetter interface {
	Fd() uintptr
}

func stat(f fs.File, t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64, err error) {
	d := t.Sys().(*syscall.Win32FileAttributeData)
	atimeNsec = d.LastAccessTime.Nanoseconds()
	mtimeNsec = d.LastWriteTime.Nanoseconds()
	ctimeNsec = d.CreationTime.Nanoseconds()

	of, ok := f.(fdGetter)
	if !ok {
		return
	}

	handle := of.Fd()
	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(syscall.Handle(handle), &info); err != nil {
		return
	}
	nlink = uint64(info.NumberOfLinks)
	return
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	// TODO: VolumeSerialNumber, FileIndexHigh and FileIndexLow are used in
	// os.SameFile, but the fields aren't exported or accessible in os.FileInfo
	// When we make our file type, get these from GetFileInformationByHandle.
	// Note that this requires access to the underlying FD number.
	return 0, 0
}
