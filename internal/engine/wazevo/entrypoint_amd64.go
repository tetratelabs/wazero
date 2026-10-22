package wazevo

import _ "unsafe"

// entrypointAsm is implemented by the backend.
//
//go:linkname entrypointAsm github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/amd64.entrypoint
func entrypointAsm(preambleExecutable, functionExecutable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr)

// afterGoFunctionCallEntrypointAsm is implemented by the backend.
//
//go:linkname afterGoFunctionCallEntrypointAsm github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/amd64.afterGoFunctionCallEntrypoint
func afterGoFunctionCallEntrypointAsm(executable *byte, executionContextPtr uintptr, stackPointer, framePointer uintptr)
