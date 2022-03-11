package jit

import (
	"reflect"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc   = kernel32.NewProc("VirtualAlloc")
	procVirtualProtect = kernel32.NewProc("VirtualProtect")
	procVirtualFree    = kernel32.NewProc("VirtualFree")
)

const (
	windows_MEM_COMMIT             = 0x00001000
	windows_MEM_RELEASE            = 0x00008000
	windows_PAGE_READWRITE         = 0x00000004
	windows_PAGE_EXECUTE_READ      = 0x00000020
	windows_PAGE_EXECUTE_READWRITE = 0x00000040
)

func mmapCodeSegment(code []byte) ([]byte, error) {
	if runtime.GOARCH == "amd64" {
		return mmapCodeSegmentAMD64(code)
	} else {
		return mmapCodeSegmentARM64(code)
	}
}

func munmapCodeSegment(code []byte) error {
	// size must be 0 because we're using MEM_RELEASE.
	// See https://docs.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtualfree
	return virtualFree(uintptr(unsafe.Pointer(&code[0])), 0, windows_MEM_RELEASE)
}

func virtualAlloc(address uintptr, size uintptr, alloctype uint32, protect uint32) (uintptr, error) {
	r0, _, err := procVirtualAlloc.Call(address, size, uintptr(alloctype), uintptr(protect))
	if r0 == 0 {
		return 0, err
	}
	return r0, nil
}

func virtualProtect(address uintptr, size uintptr, newprotect uint32, oldprotect *uint32) error {
	r1, _, e1 := procVirtualProtect.Call(address, size, uintptr(newprotect), uintptr(unsafe.Pointer(oldprotect)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func virtualFree(address uintptr, size uintptr, freetype uint32) error {
	r1, _, e1 := procVirtualFree.Call(address, size, uintptr(freetype))
	// 0 indicates failure.
	// See https://docs.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtualfree#return-value
	if r1 == 0 {
		return e1
	}
	return nil
}

func mmapCodeSegmentAMD64(code []byte) ([]byte, error) {
	p, err := virtualAlloc(0, uintptr(len(code)), windows_MEM_COMMIT, windows_PAGE_EXECUTE_READWRITE)
	if err != nil {
		return nil, err
	}

	var mem []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&mem))
	sh.Data = p
	sh.Len = len(code)
	sh.Cap = len(code)
	copy(mem, code)
	return mem, nil
}

func mmapCodeSegmentARM64(code []byte) ([]byte, error) {
	p, err := virtualAlloc(0, uintptr(len(code)), windows_MEM_COMMIT, windows_PAGE_READWRITE)
	if err != nil {
		return nil, err
	}

	var mem []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&mem))
	sh.Data = p
	sh.Len = len(code)
	sh.Cap = len(code)
	copy(mem, code)

	old := uint32(windows_PAGE_READWRITE)
	err = virtualProtect(p, uintptr(len(code)), windows_PAGE_EXECUTE_READ, &old)
	if err != nil {
		return nil, err
	}
	return mem, nil
}
