//go:build !tinygo

package sysfs

import (
	"os"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

// Link implements the same method as documented on sys.FS
func (d *dirFS) Link(oldName, newName string) experimentalsys.Errno {
	err := os.Link(d.join(oldName), d.join(newName))
	return experimentalsys.UnwrapOSError(err)
}
