package jit

import "syscall"

// Copy the code into the executable region
// and returns the byte slice of the region.
func mmapCodeSegment(code []byte) ([]byte, error) {
	mmapFunc, err := syscall.Mmap(
		-1,
		0,
		len(code),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC, syscall.MAP_PRIVATE|mmapFlags,
	)
	if err != nil {
		return nil, err
	}
	copy(mmapFunc, code)
	return mmapFunc, nil
}
