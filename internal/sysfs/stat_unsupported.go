//go:build (!((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)) || js

package sysfs

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/sys"
)

// Note: go:build constraints must be the same as /sys.stat_unsupported.go for
// the same reasons.

// dirNlinkIncludesDot might be true for some operating systems, which can have
// new stat_XX.go files as necessary.
//
// Note: this is only used in tests
const dirNlinkIncludesDot = false

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
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino, 0
	}
	return 0, 0
}
