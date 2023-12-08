//go:build tinygo

package sysfs

import (
	"io/fs"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/sys"
)

// inoFromFileInfo uses stat to get the inode information of the file.
func inoFromFileInfo(dirPath string, info fs.FileInfo) (ino sys.Inode, errno experimentalsys.Errno) {
	return 0, 0
}
