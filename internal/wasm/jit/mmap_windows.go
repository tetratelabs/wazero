package jit

import (
	"errors"
	"fmt"
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
	procGetLastError   = kernel32.NewProc("GetLastError")
)

const (
	windows_MEM_COMMIT             uintptr = 0x00001000
	windows_MEM_RELEASE            uintptr = 0x00008000
	windows_PAGE_READWRITE         uintptr = 0x00000004
	windows_PAGE_EXECUTE_READ      uintptr = 0x00000020
	windows_PAGE_EXECUTE_READWRITE uintptr = 0x00000040
)

func mmapCodeSegment(code []byte) ([]byte, error) {
	if len(code) == 0 {
		panic(errors.New("BUG: mmapCodeSegment with zero length"))
	}
	if runtime.GOARCH == "amd64" {
		return mmapCodeSegmentAMD64(code)
	} else {
		return mmapCodeSegmentARM64(code)
	}
}

func munmapCodeSegment(code []byte) error {
	if len(code) == 0 {
		panic(errors.New("BUG: munmapCodeSegment with zero length"))
	}
	return freeMemory(code)
}

// allocateMemory commits the memory region via the "VirtualAlloc" function.
// See https://docs.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtualalloc
func allocateMemory(code []byte, protect uintptr) (uintptr, error) {
	address := uintptr(0) // TODO: document why zero
	size := uintptr(len(code))
	alloctype := windows_MEM_COMMIT
	if r, _, _ := procVirtualAlloc.Call(address, size, alloctype, protect); r == 0 {
		return 0, fmt.Errorf("jit: VirtualAlloc error: %w", getLastError())
	} else {
		return r, nil
	}
}

// freeMemory releases the memory region via the "VirtualFree" function.
// See https://docs.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtualfree
func freeMemory(code []byte) error {
	address := uintptr(unsafe.Pointer(&code[0]))
	size := uintptr(0) // size must be 0 because we're using MEM_RELEASE.
	freetype := windows_MEM_RELEASE
	if r, _, _ := procVirtualFree.Call(address, size, freetype); r == 0 {
		return fmt.Errorf("jit: VirtualFree error: %w", getLastError())
	}
	return nil
}

func virtualProtect(address, size, newprotect uintptr, oldprotect *uint32) error {
	if r, _, _ := procVirtualProtect.Call(address, size, newprotect, uintptr(unsafe.Pointer(oldprotect))); r == 0 {
		return fmt.Errorf("jit: VirtualProtect error: %w", getLastError())
	}
	return nil
}

func mmapCodeSegmentAMD64(code []byte) ([]byte, error) {
	p, err := allocateMemory(code, windows_PAGE_EXECUTE_READWRITE)
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
	p, err := allocateMemory(code, windows_PAGE_READWRITE)
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

// getLastError casts the last error on the calling thread to a syscall.Errno or returns syscall.EINVAL.
//
// See https://docs.microsoft.com/en-us/windows/win32/api/errhandlingapi/nf-errhandlingapi-getlasterror
// See https://docs.microsoft.com/en-us/windows/win32/debug/system-error-codes
func getLastError() syscall.Errno {
	if errno, _, _ := procGetLastError.Call(); errno != 0 {
		return syscall.Errno(errno)
	}
	return syscall.EINVAL
}
