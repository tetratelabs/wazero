//go:build !windows && !js && !illumos && !solaris

package sysfs

import (
	"io/fs"
	"os"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// OpenFile is like os.OpenFile except it returns sys.Errno. A zero
// sys.Errno is success.
func openFile(path string, flag int, perm fs.FileMode) (*os.File, sys.Errno) {
	f, err := os.OpenFile(path, flag, perm)
	// Note: This does not return a fsapi.File because fsapi.FS that returns
	// one may want to hide the real OS path. For example, this is needed for
	// pre-opens.
	return f, sys.UnwrapOSError(err)
}
