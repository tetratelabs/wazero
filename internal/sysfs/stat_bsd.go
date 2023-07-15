//go:build (amd64 || arm64) && (darwin || freebsd)

package sysfs

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/sys"
)

// dirNlinkIncludesDot is true because even though os.File filters out dot
// entries, the underlying syscall.Stat includes them.
//
// Note: this is only used in tests
const dirNlinkIncludesDot = true

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

func inoFromFileInfo(_ string, info fs.FileInfo) (sys.Inode, syscall.Errno) {
	switch v := info.Sys().(type) {
	case *sys.Stat_t:
		return v.Ino, 0
	case *syscall.Stat_t:
		return v.Ino, 0
	default:
		return 0, 0
	}
}
