//go:build illumos || solaris

package sysfs

import (
	"io/fs"
	"os"

	"github.com/tetratelabs/wazero/experimental/sys"
)

func openFile(path string, flag int, perm fs.FileMode) (*os.File, sys.Errno) {
	f, err := os.OpenFile(path, flag, perm)
	return f, sys.UnwrapOSError(err)
}
