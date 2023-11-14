//go:build !windows

package fstest

import "io/fs"

func timesFromFileInfo(fs.FileInfo) (atim, mtime int64) {
	panic("unexpected")
}
