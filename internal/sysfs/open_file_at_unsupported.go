//go:build (!linux && !windows && !darwin) || tinygo

package sysfs

import (
	"io/fs"
	"os"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func openFileAt(dir *os.File, path string, oflag sys.Oflag, perm fs.FileMode) (*os.File, sys.Errno) {
	return nil, sys.ENOSYS
}
