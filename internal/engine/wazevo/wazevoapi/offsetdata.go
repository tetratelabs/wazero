package wazevoapi

import "github.com/tetratelabs/wazero/internal/wasm"

const (
	// FunctionInstanceSize is the size of wazevo.functionInstance.
	FunctionInstanceSize = 24
	// FunctionInstanceExecutableOffset is an offset of `executable` field in wazevo.functionInstance
	FunctionInstanceExecutableOffset = 0
	// FunctionInstanceModuleContextOpaquePtrOffset is an offset of `moduleContextOpaquePtr` field in wazevo.functionInstance
	FunctionInstanceModuleContextOpaquePtrOffset = 8
	// FunctionInstanceTypeIDOffset is an offset of `typeID` field in wazevo.functionInstance
	FunctionInstanceTypeIDOffset = 16
)

var ExecutionContextOffsets = ExecutionContextOffsetData{
	ExitCodeOffset:                          0,
	CallerModuleContextPtr:                  8,
	OriginalFramePointer:                    16,
	OriginalStackPointer:                    24,
	GoReturnAddress:                         32,
	StackBottomPtr:                          40,
	GoCallReturnAddress:                     48,
	StackPointerBeforeGrow:                  56,
	StackGrowRequiredSize:                   64,
	MemoryGrowTrampolineAddress:             72,
	SavedRegistersBegin:                     80,
	GoFunctionCallCalleeModuleContextOpaque: 1104,
	GoFunctionCallStackBegin:                1112,
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
	// GoCallReturnAddress is an offset of `stackPointerBeforeGoCall` field in wazevo.executionContext
	StackPointerBeforeGrow Offset
	// StackGrowRequiredSize is an offset of `stackGrowRequiredSize` field in wazevo.executionContext
	StackGrowRequiredSize Offset
	// MemoryGrowTrampolineAddress is an offset of `memoryGrowTrampolineAddress` field in wazevo.executionContext
	MemoryGrowTrampolineAddress Offset
	// GoCallReturnAddress is an offset of the first element of `savedRegisters` field in wazevo.executionContext
	SavedRegistersBegin Offset
	// GoFunctionCallCalleeModuleContextOpaque is an offset of `goFunctionCallCalleeModuleContextOpaque` field in wazevo.executionContext
	GoFunctionCallCalleeModuleContextOpaque Offset
	// GoFunctionCallStackBegin is an offset of the first element of `goFunctionCallStack` field in wazevo.executionContext
	GoFunctionCallStackBegin Offset
}

// ModuleContextOffsetData allows the compilers to get the information about offsets to the fields of wazevo.moduleContextOpaque,
// This is unique per module.
type ModuleContextOffsetData struct {
	TotalSize int
	ModuleInstanceOffset,
	LocalMemoryBegin,
	ImportedMemoryBegin,
	ImportedFunctionsBegin,
	GlobalsBegin,
	TypeIDs1stElement,
	TablesBegin Offset
}

// ImportedFunctionOffset returns an offset of the i-th imported function.
// Each item is stored as wazevo.functionInstance whose size matches FunctionInstanceSize.
func (m *ModuleContextOffsetData) ImportedFunctionOffset(i wasm.Index) (
	executableOffset, moduleCtxOffset, typeIDOffset Offset,
) {
	base := m.ImportedFunctionsBegin + Offset(i)*FunctionInstanceSize
	return base, base + 8, base + 16
}

// GlobalInstanceOffset returns an offset of the i-th global instance.
func (m *ModuleContextOffsetData) GlobalInstanceOffset(i wasm.Index) Offset {
	return m.GlobalsBegin + Offset(i)*8
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

// U64 encodes an Offset as int64 for convenience.
func (o Offset) U64() uint64 {
	return uint64(o)
}

// LocalMemoryBase returns an offset of the first byte of the local memory.
func (m *ModuleContextOffsetData) LocalMemoryBase() Offset {
	return m.LocalMemoryBegin
}

// LocalMemoryLen returns an offset of the length of the local memory buffer.
func (m *ModuleContextOffsetData) LocalMemoryLen() Offset {
	if l := m.LocalMemoryBegin; l >= 0 {
		return l + 8
	}
	return -1
}

// TableOffset returns an offset of the i-th table instance.
func (m *ModuleContextOffsetData) TableOffset(tableIndex int) Offset {
	return m.TablesBegin + Offset(tableIndex)*8
}

// NewModuleContextOffsetData creates a ModuleContextOffsetData determining the structure of moduleContextOpaque for the given Module.
// The structure is described in the comment of wazevo.moduleContextOpaque.
func NewModuleContextOffsetData(m *wasm.Module) ModuleContextOffsetData {
	ret := ModuleContextOffsetData{}
	var offset Offset

	ret.ModuleInstanceOffset = 0
	offset += 8

	if m.MemorySection != nil {
		ret.LocalMemoryBegin = offset
		// buffer base + memory size.
		const localMemorySizeInOpaqueModuleContext = 16
		offset += localMemorySizeInOpaqueModuleContext
	} else {
		// Indicates that there's no local memory
		ret.LocalMemoryBegin = -1
	}

	if m.ImportMemoryCount > 0 {
		// *wasm.MemoryInstance + imported memory's owner (moduleContextOpaque)
		const importedMemorySizeInOpaqueModuleContext = 16
		ret.ImportedMemoryBegin = offset
		offset += importedMemorySizeInOpaqueModuleContext
	} else {
		// Indicates that there's no imported memory
		ret.ImportedMemoryBegin = -1
	}

	if m.ImportFunctionCount > 0 {
		ret.ImportedFunctionsBegin = offset
		// Each function is stored wazevo.functionInstance.
		size := int(m.ImportFunctionCount) * FunctionInstanceSize
		offset += Offset(size)
	} else {
		ret.ImportedFunctionsBegin = -1
	}

	if globals := int(m.ImportGlobalCount) + len(m.GlobalSection); globals > 0 {
		ret.GlobalsBegin = offset
		// Pointers to *wasm.GlobalInstance.
		offset += Offset(globals) * 8
	} else {
		ret.GlobalsBegin = -1
	}

	if tables := len(m.TableSection) + int(m.ImportTableCount); tables > 0 {
		ret.TypeIDs1stElement = offset
		offset += 8 // First element of TypeIDs.

		ret.TablesBegin = offset
		// Pointers to *wasm.TableInstance.
		offset += Offset(tables) * 8
	} else {
		ret.TypeIDs1stElement = -1
		ret.TablesBegin = -1
	}

	ret.TotalSize = int(offset)
	return ret
}
