//go:build !windows
// +build !windows

package jit

import "syscall"

// mmapCodeSegment copies the code into the executable region and returns the byte slice of the region.
// See https://man7.org/linux/man-pages/man2/mmap.2.html for mmap API and flags.
func mmapCodeSegment(code []byte) ([]byte, error) {
	mmapFunc, err := syscall.Mmap(
		-1,
		0,
		len(code),
		// The region must be RWX: RW for writing native codes, X for executing the region.
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		// Anonymous as this is not an actual file, but a memory,
		// Private as this is in-process memory region.
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, err
	}
	copy(mmapFunc, code)
	return mmapFunc, nil
}
