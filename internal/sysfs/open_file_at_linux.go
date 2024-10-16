//go:build !tinygo

package sysfs

import (
	"io/fs"
	"os"
	"path"
	"syscall"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func openFileAt(dir *os.File, filePath string, oflag sys.Oflag, perm fs.FileMode) (*os.File, sys.Errno) {
	fd, err := syscall.Openat(int(dir.Fd()), filePath, toOsOpenFlag(oflag), uint32(perm))
	if err != nil {
		return nil, sys.UnwrapOSError(err)
	}

	return os.NewFile(uintptr(fd), path.Base(filePath)), 0
}
