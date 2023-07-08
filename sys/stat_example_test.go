package sys_test

import (
	"io/fs"

	"github.com/tetratelabs/wazero/sys"
)

var (
	epochNanos sys.EpochNanos
	walltime   sys.Walltime
	info       fs.FileInfo
)

func Example_epochNanos() {
	// convert sys.Walltime to EpochNanos
	sec, nsec := walltime()
	epochNanos = sec*1e9 + int64(nsec)

	// convert time.Time to EpochNanos
	epochNanos = info.ModTime().UnixNano()
}
