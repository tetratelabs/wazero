package platform

import (
	"io"
	"io/fs"
	"syscall"
)

// readdirnamesFile is implemented by os.File in dir.go
// Note: we use this until we finalize our own FD-scoped file.
type readdirnamesFile interface {
	Readdirnames(n int) (names []string, err error)
}

// Readdirnames is like the function on os.File, but for fs.File. This returns
// syscall.ENOTDIR if not a directory or syscall.EIO if closed or read
// redundantly.
func Readdirnames(f fs.File, n int) (names []string, err error) {
	switch f := f.(type) {
	case readdirnamesFile:
		names, err = f.Readdirnames(n)
	case fs.ReadDirFile:
		var entries []fs.DirEntry
		entries, err = f.ReadDir(n)
		if err != nil {
			break
		}
		names = make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
	default:
		err = syscall.ENOTDIR
	}
	err = UnwrapOSError(err)
	return
}

// Dirent is an entry read from a directory.
//
// This is a portable variant of syscall.Dirent containing fields needed for
// WebAssembly ABI including WASI snapshot-01 and wasi-filesystem. Unlike
// fs.DirEntry, this may include the Ino.
type Dirent struct {
	// Name is the base name of the directory entry.
	Name string

	// Ino is the file serial number, or zero if not available.
	Ino uint64

	// Type is fs.FileMode masked on fs.ModeType. For example, zero is a
	// regular file, fs.ModeDir is a directory and fs.ModeIrregular is unknown.
	Type fs.FileMode
}

// IsDir returns true if the Type is fs.ModeDir.
func (d *Dirent) IsDir() bool {
	return d.Type == fs.ModeDir
}

// Readdir is like the function on os.File, but for fs.File. This returns
// syscall.ENOTDIR if not a directory or syscall.EIO if closed or read
// redundantly.
func Readdir(f fs.File, n int) (dirents []*Dirent, err error) {
	switch f := f.(type) {
	case fs.ReadDirFile:
		var entries []fs.DirEntry
		entries, err = f.ReadDir(n)
		if err == io.EOF {
			err = nil
		}
		if err != nil {
			break
		}
		dirents = make([]*Dirent, 0, len(entries))
		for _, e := range entries {
			// By default, we don't attempt to read inode data
			dirents = append(dirents, &Dirent{Name: e.Name(), Type: e.Type()})
		}
	default:
		err = syscall.ENOTDIR
	}
	err = UnwrapOSError(err)
	return
}
