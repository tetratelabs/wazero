//go:build !tinygo

package sysfs

import (
	"io/fs"
	"os"
	"path"
	"strings"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

// Link implements the same method as documented on sys.FS
func (d *dirFS) Link(oldName, newName string) experimentalsys.Errno {
	err := os.Link(d.join(oldName), d.join(newName))
	return experimentalsys.UnwrapOSError(err)
}

// Unlink implements the same method as documented on sys.FS
func (d *dirFS) Unlink(path string) (err experimentalsys.Errno) {
	return unlink(d.join(path))
}

// Rename implements the same method as documented on sys.FS
func (d *dirFS) Rename(from, to string) experimentalsys.Errno {
	from, to = d.join(from), d.join(to)
	return rename(from, to)
}

// Chmod implements the same method as documented on sys.FS
func (d *dirFS) Chmod(path string, perm fs.FileMode) experimentalsys.Errno {
	err := os.Chmod(d.join(path), perm)
	return experimentalsys.UnwrapOSError(err)
}

// Symlink implements the same method as documented on sys.FS
func (d *dirFS) Symlink(oldName, link string) experimentalsys.Errno {
	// Note: do not resolve `oldName` relative to this dirFS.
	// The link result is always resolved on usage (e.g. readlink, read, etc).
	// However, this trivially allows escaping the root mount point.
	// See: https://github.com/tetratelabs/wazero/issues/2321
	// So, lexically clean `oldName`, and enforce that it always points down
	// the hierarchy.
	oldName = path.Clean(oldName)
	if strings.HasPrefix(oldName, "../") || strings.HasPrefix(oldName, "/") {
		return experimentalsys.EFAULT
	}
	err := os.Symlink(oldName, d.join(link))
	return experimentalsys.UnwrapOSError(err)
}
