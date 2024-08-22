//go:build !tinygo

package sysfs

import "os"

func truncate(file *os.File, size int64) error {
	return file.Truncate(size)
}
