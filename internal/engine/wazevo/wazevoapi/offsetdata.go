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
type ModuleContextOffsetData struct{}

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

// NewModuleContextOffsetData creates a ModuleContextOffsetData for the given Module.
func NewModuleContextOffsetData(_ *wasm.Module) ModuleContextOffsetData {
	// TODO
	return ModuleContextOffsetData{}
}

// Size returns the size of total bytes of the moduleContextOpaque.
func (m *ModuleContextOffsetData) Size() int {
	// TODO:
	return 0
}
