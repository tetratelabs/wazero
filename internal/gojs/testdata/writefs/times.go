//go:build !js

package writefs

import (
	"io/fs"
	"os"

	"github.com/tetratelabs/wazero/internal/platform"
)

func Stat(f fs.File, t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink, dev, inode uint64, err error) {
	return platform.Stat(f, t)
}
