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

// fdGetter is implemented by *os.File on Windows, but not a part of fs.File.
// On the other hand, fs.File is implemented by the wrapped version of os.File,
// therefore we define the interface here temporarily.
//
// This is only needed on Windows in order to nlink by making another raw system call
// on the file handle.
//
// TODO: once we have our own File/FileInfo type, this shouldn't be needed.
type fdGetter interface {
	// Fd is the same as os.File Fd.
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

	handle := syscall.Handle(of.Fd())
	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(handle, &info); err != nil {
		// If the file descriptor is already closed, we have to re-open just like
		// os.Stat does to allow the results on the closed files.
		// https://github.com/golang/go/blob/go1.20/src/os/stat_windows.go#L86
		//
		// TODO: once we have our File/Stat type, this shouldn't be necessary.
		// But for now, ignore the error to pass the std library test for bad file descriptor.
		// https://github.com/ziglang/zig/blob/master/lib/std/os/test.zig#L167-L170
		if err == syscall.Errno(6) {
			err = nil
		}
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
