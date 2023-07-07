//go:build (amd64 || arm64 || riscv64) && linux

// Note: This expression is not the same as compiler support, even if it looks
// similar. Platform functions here are used in interpreter mode as well.

package sysfs

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/sys"
)

func lstat(path string) (sys.Stat_t, syscall.Errno) {
	if info, err := os.Lstat(path); err != nil {
		return sys.Stat_t{}, platform.UnwrapOSError(err)
	} else {
		return sys.NewStat_t(info), 0
	}
}

func stat(path string) (sys.Stat_t, syscall.Errno) {
	if info, err := os.Stat(path); err != nil {
		return sys.Stat_t{}, platform.UnwrapOSError(err)
	} else {
		return sys.NewStat_t(info), 0
	}
}

func statFile(f fs.File) (sys.Stat_t, syscall.Errno) {
	return defaultStatFile(f)
}

func inoFromFileInfo(_ string, t fs.FileInfo) (ino sys.Ino, err syscall.Errno) {
	if d, ok := t.Sys().(*syscall.Stat_t); ok {
		ino = d.Ino
	}
	return
}
