//go:build tinygo

package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
)

func unlink(name string) (errno sys.Errno) {
	return 0
}
