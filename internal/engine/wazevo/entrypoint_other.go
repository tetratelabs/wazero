//go:build !(arm64 || amd64)

package wazevo

import (
	"runtime"
)

func entrypointAsm(preambleExecutable, functionExecutable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr) {
	panic(runtime.GOARCH)
}

func afterGoFunctionCallEntrypointAsm(executable *byte, executionContextPtr uintptr, stackPointer, framePointer uintptr) {
	panic(runtime.GOARCH)
}
