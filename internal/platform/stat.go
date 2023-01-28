package platform

import "os"

// Stat returns platform-specific values if os.FileInfo Sys is available.
// Otherwise, it returns the mod time for all values.
func Stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	if t.Sys() == nil { // possibly fake filesystem
		atimeNsec, mtimeNsec, ctimeNsec = mtimes(t)
		nlink = 1
		return
	}
	return stat(t)
}

// StatDeviceInode returns platform-specific values if os.FileInfo Sys is
// available. Otherwise, it returns zero which makes file identity comparison
// unsupported.
//
// Returning zero for now works in most cases, except notably wasi-libc
// code that needs to compare file identity via the underlying data as
// opposed to a host function similar to os.SameFile.
// See https://github.com/WebAssembly/wasi-filesystem/issues/65
func StatDeviceInode(t os.FileInfo) (dev, inode uint64) {
	if t.Sys() == nil { // possibly fake filesystem
		return
	}
	return statDeviceInode(t)
}

func mtimes(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64) {
	mtimeNsec = t.ModTime().UnixNano()
	atimeNsec = mtimeNsec
	ctimeNsec = mtimeNsec
	return
}
