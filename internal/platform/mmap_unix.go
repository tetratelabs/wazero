// Separated from linux which has support for huge pages.
//go:build unix && !linux

package platform

import "syscall"

func mmapCodeSegment(size int, exec bool) ([]byte, error) {
	// Anonymous as this is not an actual file, but a memory,
	// Private as this is in-process memory region.
	flags := syscall.MAP_ANON | syscall.MAP_PRIVATE
	prot := syscall.PROT_READ | syscall.PROT_WRITE
	if exec {
		prot |= syscall.PROT_EXEC
	}
	return syscall.Mmap(-1, 0, size, prot, flags)
}

func munmapCodeSegment(code []byte) error {
	return syscall.Munmap(code)
}
