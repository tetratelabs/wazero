package platform

import (
	"syscall"
	"unsafe"
)

const (
	// https://man7.org/linux/man-pages/man2/mremap.2.html
	__MREMAP_MAYMOVE = 1
	__MREMAP_FIXED   = 2
)

func remapCodeSegmentAMD64(code []byte, size int) ([]byte, error) {
	return remapCodeSegment(code, size, mmapProtAMD64)
}

func remapCodeSegmentARM64(code []byte, size int) ([]byte, error) {
	return remapCodeSegment(code, size, mmapProtARM64)
}

func remapCodeSegment(code []byte, size, prot int) ([]byte, error) {
	b, err := mmapCodeSegment(size, prot)
	if err != nil {
		return nil, err
	}
	oldAddr := *(*unsafe.Pointer)(unsafe.Pointer(&code))
	newAddr := *(*unsafe.Pointer)(unsafe.Pointer(&b))
	_, err = mremap(oldAddr, len(code), size, __MREMAP_MAYMOVE|__MREMAP_FIXED, newAddr)
	if err != nil {
		mustMunmapCodeSegment(b)
		return nil, err
	}
	return b, nil
}

//go:nosplit
func mremap(oldAddr unsafe.Pointer, oldSize, newSize, flags int, newAddr unsafe.Pointer) (unsafe.Pointer, error) {
	p, _, err := syscall.Syscall6(
		syscall.SYS_MREMAP,
		uintptr(oldAddr),
		uintptr(oldSize),
		uintptr(newSize),
		uintptr(flags),
		uintptr(newAddr),
		uintptr(0),
	)
	if err != 0 {
		return nil, err
	}
	return unsafe.Pointer(p), nil
}
