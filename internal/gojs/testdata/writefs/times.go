//go:build !js

package writefs

import (
	"os"

	"github.com/tetratelabs/wazero/internal/platform"
)

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	return platform.Stat(t) // allow the file to compile and run outside JS
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	return platform.StatDeviceInode(t)
}
