package platform

import (
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
		if err == nil {
			names = make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
		}
	default:
		err = syscall.ENOTDIR
	}
	err = UnwrapOSError(err)
	return
}
