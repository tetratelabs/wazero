//go:build !js

package writefs

import (
	"os"

	"github.com/tetratelabs/wazero/internal/platform"
)

func statTimes(t os.FileInfo) (atimeSec, atimeNsec, mtimeSec, mtimeNsec, ctimeSec, ctimeNsec int64) {
	return platform.StatTimes(t) // allow the file to compile and run outside JS
}
