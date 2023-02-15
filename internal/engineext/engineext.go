// Package engineext must be aligned with interfaced in the wasm/engine.go,
// and should not depend on any internal package so that this could be
// implemented out-of-source engine implementations.
//
// This package is supposed to be copied in the out-of-source engine implementation's repository
package engineext

import (
	"context"
	"crypto/sha256"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

type (
	// Index is the same as wasm.Index.
	Index = uint32
	// FunctionTypeID is the same as wasm.FunctionTypeID.
	FunctionTypeID = uint32
	// Reference is the same as wasm.Reference.
	Reference = uintptr
	// ModuleID is the same as wasm.ModuleID.
	ModuleID = [sha256.Size]byte
)

// MemoryInstanceBufferOffset is the offset of wasm.MemoryInstance Buffer.
const MemoryInstanceBufferOffset = 0

// EngineExt is the counterpart of wasm.Engine, but defined as a struct where each method is defined as function.
// This way, the same struct can be implemented in out-of-source repository by copying the struct definition.
// Note that we shouldn't use any non-builtin types. Otherwise, such copy-and-reuse trick doesn't work.
type EngineExt struct {
	// CloseFn is the same as wasm.Engine CloseFn except this doesn't contain non-build types.
	CloseFn func() (err error)
	// CompileModuleFn is the same as wasm.Engine CompileModuleFn except this doesn't contain non-builtin types.
	CompileModuleFn func(ctx context.Context, module any, listeners []experimental.FunctionListener, ensureTermination bool) error
	// CompiledModuleCountFn is the same as wasm.Engine CompiledModuleCountFn except this doesn't contain non-builtin types.
	CompiledModuleCountFn func() uint32
	// DeleteCompiledModuleFn is the same as wasm.Engine DeleteCompiledModuleFn except this doesn't contain non-builtin types.
	DeleteCompiledModuleFn func(module any)
	// NewModuleEngineFn is the same as wasm.Engine NewModuleEngineFn except this doesn't contain non-builtin types.
	NewModuleEngineFn func(name string, module any, moduleInst any) (ModuleEngineExt, error)
}

// ModuleEngineExt is the counterpart of wasm.ModuleEngine, but defined as a struct where each method is defined as function.
// This way, the same struct can be implemented in out-of-source repository by copying the struct definition.
// Note that we shouldn't use any non-builtin types. Otherwise, such copy-and-reuse trick doesn't work.
type ModuleEngineExt struct {
	// NameFn is the same as wasm.ModuleEngine except this doesn't contain non-builtin types.
	NameFn func() string
	// NewCallEngineFn is the same as wasm.ModuleEngine except this doesn't contain non-builtin types.
	NewCallEngineFn func(callCtx any, f any) (CallEngineExt, error)
	// LookupFunctionFn is the same as wasm.ModuleEngine except this doesn't contain non-builtin types.
	LookupFunctionFn func(t any, typeId FunctionTypeID, tableOffset Index) (Index, error)
	// GetFunctionReferencesFn is the same as wasm.ModuleEngine except this doesn't contain non-builtin types.
	GetFunctionReferencesFn func(indexes []*Index) []Reference
	// FunctionInstanceReferenceFn is the same as wasm.ModuleEngine except this doesn't contain non-builtin types.
	FunctionInstanceReferenceFn func(funcIndex Index) Reference
}

// CallEngineExt is the counterpart of wasm.CallEngine but without having non-builtin types.
type CallEngineExt = func(ctx context.Context, m any, params []uint64) (results []uint64, err error)

// Module is implemented by *wasm.Module. In the out-of-source implementations, we cast `any` to this interface.
// If we directly use this interface in the signature directly, Go runtime treats this and copied interface differently
// and makes it impossible to insert the custom EngineExt, ModuleEngineExt, etc.
type Module interface {
	// ModuleID returns ModuleID of this module.
	ModuleID() ModuleID
	// TypeCounts returns the number of types defined in this module.
	TypeCounts() uint32
	// Type returns the function type for the index `i`.
	Type(i Index) (params, results []api.ValueType)
	// FuncTypeIndex returns the type index of the function at `funcIndex`.
	FuncTypeIndex(funcIndex Index) (typeIndex Index)
	// HostModule returns true if this is a host module.
	HostModule() bool
	// ImportFuncCount returns the imported function count.
	ImportFuncCount() uint32
	// LocalMemoriesCount returns the local memories count. Currently, 0 or 1.
	LocalMemoriesCount() uint32
	// ImportedMemoriesCount returns the imported memory count. Currently, 0 or 1.
	ImportedMemoriesCount() uint32
	// MemoryMinMax returns min and max of the defined memory with ok=true. Returns ok=false if memory not exists.
	MemoryMinMax() (min, max uint32, ok bool)
	// CodeCount returns the number of codes (number of locally defined functions) in this module.
	CodeCount() uint32
	// CodeAt returns the local types and body of the function at `i`.
	CodeAt(i Index) (localTypes, body []byte)
}

// ModuleInstance is implemented by *wasm.ModuleInstance. In the out-of-source implementations, we cast `any` param to this interface.
// If we directly use this interface in the signature directly, Go runtime treats this and copied interface differently
// and makes it impossible to insert the custom EngineExt, ModuleEngineExt, etc.
type ModuleInstance interface {
	// ModuleInstanceName returns wasm.ModuleInstance Name.
	ModuleInstanceName() string
	// ImportedFunctions returns two index-correlated imported functions.
	// - moduleInstances contains wasm.ModuleInstance for each imported function.
	// - indexes contains the index to wasm.ModuleInstance FunctionInstances.
	ImportedFunctions() (moduleInstances []any, indexes []Index)
	// MemoryInstanceBuffer returns wasm.MemoryInstance Buffer if exists.
	MemoryInstanceBuffer() []byte
	// ImportedMemoryInstancePtr returns the address (as uintptr) of wasm.MemoryInstance if exists.
	ImportedMemoryInstancePtr() uintptr
}

// FunctionInstance is implemented by *wasm.FunctionInstance. In the out-of-source implementations, we cast `any` to this interface.
// If we directly use this interface in the signature directly, Go runtime treats this and copied interface differently
// and makes it impossible to insert the custom EngineExt, ModuleEngineExt, etc.
type FunctionInstance interface {
	// ModuleInstanceName returns wasm.ModuleInstance Name of this function.
	ModuleInstanceName() string
	// FunctionType returns the signature of this function.
	FunctionType() (params []api.ValueType, results []api.ValueType)
	// Index returns wasm.FunctionInstance Idx.
	Index() Index
}
