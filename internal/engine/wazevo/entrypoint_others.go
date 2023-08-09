//go:build !arm64

package wazevo

import (
	"runtime"
)

func entrypoint(executable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr) {
	panic(runtime.GOARCH)
}

func afterStackGrowEntrypoint(executable *byte, executionContextPtr uintptr, stackPointer uintptr) {
	panic(runtime.GOARCH)
}
