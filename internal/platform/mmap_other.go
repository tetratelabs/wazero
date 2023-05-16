// This uses syscall.Mprotect. Go's SDK only supports this on darwin and linux.
//go:build darwin || freebsd

package platform

import (
	"syscall"
)

func mmapCodeSegment(size, prot int) ([]byte, error) {
	return syscall.Mmap(
		-1,
		0,
		size,
		prot,
		// Anonymous as this is not an actual file, but a memory,
		// Private as this is in-process memory region.
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
}
