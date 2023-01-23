// Package writefs includes wazero-specific fs.FS implementations that allow
// creation and deletion of files and directories.
//
// This is a work-in-progress and a workaround needed because write support is
// not yet supported in fs.FS. See https://github.com/golang/go/issues/45757
//
// Tracking issue: https://github.com/tetratelabs/wazero/issues/390
package writefs

import (
	"io/fs"

	"github.com/tetratelabs/wazero/internal/sysfs"
)

// NewDirFS creates a writeable filesystem at the given path on the host
// filesystem.
//
// This is like os.DirFS, but allows creation and deletion of files and
// directories, as well as timestamp modifications. None of which are supported
// in fs.FS.
//
// The following errors are expected:
//   - syscall.EINVAL: `dir` is invalid.
//   - syscall.ENOENT: `dir` doesn't exist.
//   - syscall.ENOTDIR: `dir` exists, but is not a directory.
//
// # Isolation
//
// Symbolic links can escape the root path as files are opened via os.OpenFile
// which cannot restrict following them.
//
// # This is wazero-only
//
// Do not attempt to use the result as a fs.FS, as it will panic. This is a
// bridge to a future filesystem abstraction made for wazero.
func NewDirFS(dir string) fs.FS {
	// sysfs.DirFS is intentionally internal as it is still evolving
	return &sysfs.FSHolder{FS: sysfs.NewDirFS(dir)}
}
