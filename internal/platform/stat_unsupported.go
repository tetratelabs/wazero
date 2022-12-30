//go:build !(darwin || linux || freebsd || windows)

package platform

import "os"

func statTimes(t os.FileInfo) (atimeSec, atimeNSec, mtimeSec, mtimeNSec, ctimeSec, ctimeNSec int64) {
	return mtimes(t)
}
