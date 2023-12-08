//go:build tinygo

package sysfs

import (
	"github.com/tetratelabs/wazero/experimental/sys"
)

func rename(from, to string) sys.Errno {
	return 0
}
