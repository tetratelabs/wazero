//go:build !js

package writefs

import (
	"os"

	"github.com/tetratelabs/wazero/internal/platform"
)

func statTimes(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64) {
	return platform.StatTimes(t) // allow the file to compile and run outside JS
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	return platform.StatDeviceInode(t)
}
