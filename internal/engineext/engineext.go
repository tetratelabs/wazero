// Package engineext must be aligned with interfaced in the wasm/engine.go,
// and should not depend on any internal package so that this could be
// implemented out-of-repository engine implementations.
package engineext

import (
	"context"
	"crypto/sha256"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

type (
	Index          = uint32
	FunctionTypeID = uint32
	Reference      = uintptr
)

const MemoryInstanceBufferOffset = 0

type EngineExt struct {
	CloseFn                func() (err error)
	CompileModuleFn        func(ctx context.Context, module any, listeners []experimental.FunctionListener, ensureTermination bool) error
	CompiledModuleCountFn  func() uint32
	DeleteCompiledModuleFn func(module any)
	NewModuleEngineFn      func(name string, module any, moduleInst any) (ModuleEngineExt, error)
}

type ModuleEngineExt struct {
	NameFn                      func() string
	NewCallEngineFn             func(callCtx any, f any) (CallEngineExt, error)
	LookupFunctionFn            func(t any, typeId FunctionTypeID, tableOffset Index) (Index, error)
	GetFunctionReferencesFn     func(indexes []*Index) []Reference
	FunctionInstanceReferenceFn func(funcIndex Index) Reference
	moduleEngine                any
}

type CallEngineExt = func(ctx context.Context, m any, params []uint64) (results []uint64, err error)

type ModuleID = [sha256.Size]byte

type Module interface {
	ModuleID() ModuleID
	TypeCounts() uint32
	Type(i Index) (params, results []api.ValueType)
	FuncTypeIndex(funcIndex Index) (typeIndex Index)
	HostModule() bool
	ImportFuncCount() uint32
	LocalMemoriesCount() uint32
	ImportedMemoriesCount() uint32
	MemoryMinMax() (min, max uint32, ok bool)
	CodeCount() uint32
	CodeAt(i Index) (localTypes, body []byte)
}

type ModuleInstance interface {
	ModuleInstanceName() string
	ImportedFunctions() (moduleInstances []any, indexes []Index)
	MemoryInstanceBuffer() []byte
	ImportedMemoryInstancePtr() uintptr
}

type FunctionInstance interface {
	ModuleInstanceName() string
	FunctionType() ([]api.ValueType, []api.ValueType)
	Index() Index
}
