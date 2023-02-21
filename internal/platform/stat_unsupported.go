//go:build !((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)

package platform

import "os"

func fillStatFromOpenFile(stat *Stat_t, fd uintptr, t os.FileInfo) (err error) {
	fillStatFromFileInfo(stat, t)
	return
}
