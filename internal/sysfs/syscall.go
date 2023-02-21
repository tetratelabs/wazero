//go:build !windows

package sysfs

import (
	"io/fs"
)

func maybeWrapFile(f file, _ FS, _ string, _ int, _ fs.FileMode) file {
	return f
}
