package wazevo

import _ "unsafe"

// entrypoint is implemented by the backend.
//
//go:linkname entrypoint github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64.entrypoint
func entrypoint(executable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultStackPtr *uint64, goAllocatedStackSlicePtr uintptr)

// entrypoint is implemented by the backend.
//
//go:linkname afterStackGrowEntrypoint github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64.afterStackGrowEntrypoint
func afterStackGrowEntrypoint(executable *byte, executionContextPtr uintptr, stackPointer uintptr)
