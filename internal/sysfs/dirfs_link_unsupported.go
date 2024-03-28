//go:build tinygo

package sysfs

import (
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

// Link implements the same method as documented on sys.FS
func (d *dirFS) Link(oldName, newName string) experimentalsys.Errno {
	return experimentalsys.ENOSYS
}
