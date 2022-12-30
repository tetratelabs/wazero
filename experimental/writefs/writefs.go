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

	"github.com/tetratelabs/wazero/internal/syscallfs"
)

// DirFS creates a writeable filesystem at the given path on the host filesystem.
//
// This is like os.DirFS, but allows creation and deletion of files and
// directories, as well as timestamp modifications. None of which are supported
// in fs.FS.
//
// # Isolation
//
// Symbolic links can escape the root path as files are opened via os.OpenFile
// which cannot restrict following them.
func DirFS(dir string) fs.FS {
	// writefs.DirFS is intentionally internal as it is still evolving
	return syscallfs.DirFS(dir)
}
