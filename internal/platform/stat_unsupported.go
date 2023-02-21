//go:build (!((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)) || js

package platform

import "os"

func stat(path string, st *Stat_t) (err error) {
	t, err := os.Stat(path)
	if err = UnwrapOSError(err); err == nil {
		fillStatFromFileInfo(st, t)
	}
	return
}

func fillStatFromOpenFile(stat *Stat_t, fd uintptr, t os.FileInfo) (err error) {
	fillStatFromFileInfo(stat, t)
	return
}
