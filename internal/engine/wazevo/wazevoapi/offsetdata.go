package wazevoapi

import "github.com/tetratelabs/wazero/internal/wasm"

var ExecutionContextOffsets = ExecutionContextOffsetData{
	ExitCodeOffset:         0,
	CallerModuleContextPtr: 8,
	OriginalFramePointer:   16,
	OriginalStackPointer:   24,
	GoReturnAddress:        32,
	StackBottomPtr:         40,
	GoCallReturnAddress:    48,
	StackPointerBeforeGrow: 56,
	StackGrowRequiredSize:  64,
	SavedRegistersBegin:    80,
}

// ExecutionContextOffsetData allows the compilers to get the information about offsets to the fields of wazevo.executionContext,
// which are necessary for compiling various instructions. This is globally unique.
type ExecutionContextOffsetData struct {
	// ExitCodeOffset is an offset of `exitCode` field in wazevo.executionContext
	ExitCodeOffset Offset
	// CallerModuleContextPtr is an offset of `callerModuleContextPtr` field in wazevo.executionContext
	CallerModuleContextPtr Offset
	// CallerModuleContextPtr is an offset of `originalFramePointer` field in wazevo.executionContext
	OriginalFramePointer Offset
	// OriginalStackPointer is an offset of `originalStackPointer` field in wazevo.executionContext
	OriginalStackPointer Offset
	// GoReturnAddress is an offset of `goReturnAddress` field in wazevo.executionContext
	GoReturnAddress Offset
	// StackBottomPtr is an offset of `stackBottomPtr` field in wazevo.executionContext
	StackBottomPtr Offset
	// GoCallReturnAddress is an offset of `goCallReturnAddress` field in wazevo.executionContext
	GoCallReturnAddress Offset
	// GoCallReturnAddress is an offset of `stackPointerBeforeGrow` field in wazevo.executionContext
	StackPointerBeforeGrow Offset
	// StackGrowRequiredSize is an offset of `stackGrowRequiredSize` field in wazevo.executionContext
	StackGrowRequiredSize Offset
	// GoCallReturnAddress is an offset of the first element of `savedRegisters` field in wazevo.executionContext
	SavedRegistersBegin Offset
}

// ModuleContextOffsetData allows the compilers to get the information about offsets to the fields of wazevo.moduleContextOpaque,
// This is unique per module.
type ModuleContextOffsetData struct {
	TotalSize                                                     int
	LocalMemoryBegin, ImportedMemoryBegin, ImportedFunctionsBegin Offset
}

func (m *ModuleContextOffsetData) ImportedFunctionOffset(i wasm.Index) (ptr, moduleCtx Offset) {
	base := m.ImportedFunctionsBegin + Offset(i)*16
	return base, base + 8
}

// Offset represents an offset of a field of a struct.
type Offset int32

// U32 encodes an Offset as uint32 for convenience.
func (o Offset) U32() uint32 {
	return uint32(o)
}

// I64 encodes an Offset as int64 for convenience.
func (o Offset) I64() int64 {
	return int64(o)
}

// NewModuleContextOffsetData creates a ModuleContextOffsetData determining the structure of moduleContextOpaque for the given Module.
// The structure is described in the comment of wazevo.moduleContextOpaque.
func NewModuleContextOffsetData(m *wasm.Module) ModuleContextOffsetData {
	ret := ModuleContextOffsetData{}
	var offset Offset
	if m.MemorySection != nil {
		ret.LocalMemoryBegin = offset
		// buffer base + memory size.
		const localMemorySizeInOpaqueVMContext = 16
		offset += localMemorySizeInOpaqueVMContext
		ret.TotalSize += localMemorySizeInOpaqueVMContext
	} else {
		// Indicates that there's no local memory
		ret.LocalMemoryBegin = -1
	}

	if m.ImportMemoryCount > 0 {
		// *wasm.MemoryInstance
		const importedMemorySizeInOpaqueVMCContext = 8
		ret.ImportedMemoryBegin = offset
		offset += importedMemorySizeInOpaqueVMCContext
		ret.TotalSize += importedMemorySizeInOpaqueVMCContext
	} else {
		// Indicates that there's no imported memory
		ret.ImportedMemoryBegin = -1
	}

	if m.ImportFunctionCount > 0 {
		ret.ImportedFunctionsBegin = offset
		// Each function consists of the pointer to the executable and the pointer to its moduleContextOpaque (16 bytes).
		ret.TotalSize += int(m.ImportFunctionCount) * 16
	} else {
		ret.ImportedFunctionsBegin = -1
	}
	return ret
}
