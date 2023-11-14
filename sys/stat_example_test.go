package sys_test

import (
	"io/fs"
	"math"

	"github.com/tetratelabs/wazero/sys"
)

var (
	walltime sys.Walltime
	info     fs.FileInfo
	st       sys.Stat_t
)

// This shows typical conversions to sys.EpochNanos type, for sys.Stat_t fields.
func Example_epochNanos() {
	// Convert an adapted fs.File's fs.FileInfo to Mtim.
	st.Mtim = info.ModTime().UnixNano()

	// Generate a fake Atim using sys.Walltime passed to wazero.ModuleConfig.
	sec, nsec := walltime()
	st.Atim = sec*1e9 + int64(nsec)
}

type fileInfoWithSys struct {
	fs.FileInfo
	st sys.Stat_t
}

func (f *fileInfoWithSys) Sys() any { return &f.st }

// This shows how to return data not defined in fs.FileInfo, notably sys.Inode.
func Example_inode() {
	st := sys.NewStat_t(info)
	st.Ino = math.MaxUint64 // arbitrary non-zero value
	info = &fileInfoWithSys{info, st}
}
