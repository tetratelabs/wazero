package platform

import (
	"syscall"
	"unsafe"
)

const (
	__MREMAP_MAYMOVE = 1
)

func remapCodeSegmentAMD64(code []byte, size int) ([]byte, error) {
	return remapCodeSegment(code, size)
}

func remapCodeSegmentARM64(code []byte, size int) ([]byte, error) {
	return remapCodeSegment(code, size)
}

func remapCodeSegment(code []byte, size int) ([]byte, error) {
	p, err := mremap(*(*unsafe.Pointer)(unsafe.Pointer(&code)), len(code), size, __MREMAP_MAYMOVE)
	if err != nil {
		return nil, err
	}
	return unsafe.Slice((*byte)(p), size), nil
}

//go:nosplit
func mremap(oldAddr unsafe.Pointer, oldSize, newSize, flags int) (unsafe.Pointer, error) {
	p, _, err := syscall.Syscall6(
		syscall.SYS_MREMAP,
		uintptr(oldAddr),
		uintptr(oldSize),
		uintptr(newSize),
		uintptr(flags),
		uintptr(0),
		uintptr(0),
	)
	if err != 0 {
		return nil, err
	}
	return unsafe.Pointer(p), nil
}
