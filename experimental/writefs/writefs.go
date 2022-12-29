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

	"github.com/tetratelabs/wazero/internal/writefs"
)

// DirFS creates a writeable filesystem at the given path on the host filesystem.
//
// This is like os.DirFS, but allows creation and deletion of files and
// directories, which aren't yet supported in fs.FS.
//
// # Isolation
//
// Symbolic links can escape the root path as files are opened via os.OpenFile
// which cannot restrict following them.
func DirFS(dir string) fs.FS {
	// writefs.FS is intentionally internal as it is still evolving
	return writefs.DirFS(dir)
}
