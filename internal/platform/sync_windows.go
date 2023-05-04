package platform

import (
	"io/fs"
	"syscall"
)

func sync(f fs.File) syscall.Errno {
	if s, ok := f.(syncFile); ok {
		errno := UnwrapOSError(s.Sync())
		// Coerce error performing stat on a directory to 0, as it won't work
		// on Windows.
		switch errno {
		case syscall.EACCES /* Go 1.20 */, syscall.EBADF /* Go 1.18 */ :
			if st, err := f.Stat(); err == nil && st.IsDir() {
				errno = 0
			}
		}
		return errno
	}
	return 0
}
