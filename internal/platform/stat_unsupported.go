//go:build (!((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)) || js

package platform

import (
	"io/fs"
	"os"
)

func lstat(path string) (Stat_t, error) {
	t, err := os.Lstat(path)
	if err = UnwrapOSError(err); err != nil {
		return statFromFileInfo(t), nil
	}
	return Stat_t{}, err
}

func stat(path string) (Stat_t, error) {
	t, err := os.Stat(path)
	if err = UnwrapOSError(err); err == nil {
		return statFromFileInfo(t), nil
	}
	return Stat_t{}, err
}

func statFile(f fs.File) (Stat_t, error) {
	return defaultStatFile(f)
}

func inoFromFileInfo(readdirFile, fs.FileInfo) (ino uint64, err error) {
	return
}

func statFromFileInfo(t fs.FileInfo) Stat_t {
	return statFromDefaultFileInfo(t)
}
