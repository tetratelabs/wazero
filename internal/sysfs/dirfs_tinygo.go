//go:build tinygo

package sysfs

import (
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

func DirFS(dir string) experimentalsys.FS {
	return nil
}
