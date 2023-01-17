//go:build !((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)

package platform

import "os"

func statTimes(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64) {
	return mtimes(t)
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	return
}
