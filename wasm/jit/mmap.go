//go:build !windows
// +build !windows

package jit

import (
	"runtime"
	"syscall"
)

// mmapCodeSegment copies the code into the executable region and returns the byte slice of the region.
// See https://man7.org/linux/man-pages/man2/mmap.2.html for mmap API and flags.
func mmapCodeSegment(code []byte) ([]byte, error) {
	if runtime.GOARCH == "amd64" {
		return mmapCodeSegmentAMD64(code)
	} else {
		return mmapCodeSegmentARM64(code)
	}
}

// On amd64, we have to give all read-write-exec permission to the mmap region
// to enter the function. Otherwise, segmentation fault exeception is raied.
func mmapCodeSegmentAMD64(code []byte) ([]byte, error) {
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

// On arm64, we cannot give all read-write-exec permission to the mmap region.
// Otherwise, the mmap systemcall would raise an error. Here we give read-write
// to the region at first, write the native code and then change the perm to
// read-exec so we can execute the native code.
func mmapCodeSegmentARM64(code []byte) ([]byte, error) {
	mmapFunc, err := syscall.Mmap(
		-1,
		0,
		len(code),
		// The region must be RW: RW for writing native codes.
		syscall.PROT_READ|syscall.PROT_WRITE,
		// Anonymous as this is not an actual file, but a memory,
		// Private as this is in-process memory region.
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, err
	}

	copy(mmapFunc, code)

	// Then we're done with writing code, change the permission to RX.
	err = syscall.Mprotect(mmapFunc, syscall.PROT_READ|syscall.PROT_EXEC)
	return mmapFunc, err
}
