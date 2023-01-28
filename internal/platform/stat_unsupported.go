//go:build !((amd64 || arm64 || riscv64) && linux) && !((amd64 || arm64) && (darwin || freebsd)) && !((amd64 || arm64) && windows)

package platform

import "os"

func stat(t os.FileInfo) (atimeNsec, mtimeNsec, ctimeNsec int64, nlink uint64) {
	atimeNsec, mtimeNsec, ctimeNsec = mtimes(t)
	return
}

func statDeviceInode(t os.FileInfo) (dev, inode uint64) {
	return
}
