//go:build (!((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)) || js

package sys

import "io/fs"

const sysParseable = false

func statFromFileInfo(t fs.FileInfo) Stat_t {
	return defaultStatFromFileInfo(t)
}
