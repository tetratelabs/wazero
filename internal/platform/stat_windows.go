//go:build (amd64 || arm64) && windows

package platform

import (
	"os"
	"syscall"
)

func stat(path string, st *Stat_t) (err error) {
	// TODO: See if we can refactor to avoid opening a file first.
	f, err := OpenFile(path, syscall.O_RDONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	return StatFile(f, st)
}

func fillStatFromOpenFile(stat *Stat_t, fd uintptr, t os.FileInfo) (err error) {
	d := t.Sys().(*syscall.Win32FileAttributeData)
	handle := syscall.Handle(fd)
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

	// FileIndex{High,Low} can be combined and used as a unique identifier like inode.
	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/ns-fileapi-by_handle_file_information
	stat.Ino = (uint64(info.FileIndexHigh) << 32) | uint64(info.FileIndexLow)
	stat.Dev = uint64(info.VolumeSerialNumber)
	stat.Mode = t.Mode()
	stat.Nlink = uint64(info.NumberOfLinks)
	stat.Size = t.Size()
	stat.Atim = d.LastAccessTime.Nanoseconds()
	stat.Mtim = d.LastWriteTime.Nanoseconds()
	stat.Ctim = d.CreationTime.Nanoseconds()
	return
}
