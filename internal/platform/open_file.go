//go:build !windows

package platform

import (
	"io/fs"
	"os"
)

func OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}
