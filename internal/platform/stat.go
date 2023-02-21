package platform

import (
	"io/fs"
	"syscall"
)

// Stat_t is similar to syscall.Stat_t, and fields frequently used by
// WebAssembly ABI including WASI snapshot-01, GOOS=js and wasi-filesystem.
//
// # Note
//
// Zero values may be returned where not available. For example, fs.FileInfo
// implementations may not be able to provide Ino values.
type Stat_t struct {
	// Dev is the device ID of device containing the file.
	Dev uint64

	// Ino is the file serial number.
	Ino uint64

	// Mode is the same as Mode on fs.FileInfo containing bits to identify the
	// type of the file and its permissions (fs.ModePerm).
	Mode fs.FileMode

	/// Nlink is the number of hard links to the file.
	Nlink uint64
	// ^^ uint64 not uint16 to accept widest syscall.Stat_t.Nlink

	// Size is the length in bytes for regular files. For symbolic links, this
	// is length in bytes of the pathname contained in the symbolic link.
	Size int64
	// ^^ int64 not uint64 to defer to fs.FileInfo

	// Atim is the last data access timestamp in epoch nanoseconds.
	Atim int64

	// Mtim is the last data modification timestamp in epoch nanoseconds.
	Mtim int64

	// Ctim is the last file status change timestamp in epoch nanoseconds.
	Ctim int64
}

// Stat is like syscall.Stat. This returns syscall.ENOENT if the path doesn't
// exist.
func Stat(path string, st *Stat_t) error {
	return stat(path, st) // extracted to override more expensively in windows
}

// StatFile is like syscall.Fstat, but for fs.File instead of a file
// descriptor. This returns syscall.EBADF if the file or directory was closed.
// Note: windows allows you to stat a closed directory.
func StatFile(f fs.File, st *Stat_t) (err error) {
	t, err := f.Stat()
	if err = UnwrapOSError(err); err != nil {
		if err == syscall.EIO { // linux/darwin returns this on a closed file.
			err = syscall.EBADF // windows returns this, which is better.
		}
		return
	}
	return fillStatFile(st, f, t)
}

// fdFile is implemented by os.File in file_unix.go and file_windows.go
// Note: we use this until we finalize our own FD-scoped file.
type fdFile interface{ Fd() (fd uintptr) }

func fillStatFile(stat *Stat_t, f fs.File, t fs.FileInfo) (err error) {
	if of, ok := f.(fdFile); !ok { // possibly fake filesystem
		fillStatFromFileInfo(stat, t)
	} else {
		err = fillStatFromOpenFile(stat, of.Fd(), t)
	}
	return
}

func fillStatFromFileInfo(stat *Stat_t, t fs.FileInfo) {
	stat.Ino = 0
	stat.Dev = 0
	stat.Mode = t.Mode()
	stat.Nlink = 1
	stat.Size = t.Size()
	mtim := t.ModTime().UnixNano() // Set all times to the mod time
	stat.Atim = mtim
	stat.Mtim = mtim
	stat.Ctim = mtim
}
