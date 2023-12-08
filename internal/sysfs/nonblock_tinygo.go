//go:build tinygo

package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
)

func setNonblock(fd uintptr, enable bool) sys.Errno {
	return 0
}

func isNonblock(f *osFile) bool {
	return false
}
