package wasm

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/heeus/hwazero/api"
	"github.com/heeus/hwazero/sys"
)

// compile time check to ensure ModuleContext implements api.Module
var _ api.Module = &ModuleContext{}

func NewModuleContext(ctx context.Context, store *Store, instance *ModuleInstance, Sys *SysContext) *ModuleContext {
	zero := uint64(0)
	return &ModuleContext{ctx: ctx, memory: instance.Memory, module: instance, store: store, Sys: Sys, closed: &zero}
}

// ModuleContext implements api.Module
type ModuleContext struct {
	// ctx is returned by Context and overridden WithContext
	ctx    context.Context
	module *ModuleInstance
	// memory is returned by Memory and overridden WithMemory
	memory api.Memory
	store  *Store

	// Note: This is a part of ModuleContext so that scope is correct and Close is coherent.
	// Sys is exposed only for WASI
	Sys *SysContext

	// closed is the pointer used both to guard moduleEngine.CloseWithExitCode and to store the exit code.
	//
	// The update value is 1 + exitCode << 32. This ensures an exit code of zero isn't mistaken for never closed.
	//
	// Note: Exclusively reading and updating this with atomics guarantees cross-goroutine observations.
	// See /RATIONALE.md
	closed *uint64
}

// FailIfClosed returns a sys.ExitError if CloseWithExitCode was called.
func (m *ModuleContext) FailIfClosed() error {
	if closed := atomic.LoadUint64(m.closed); closed != 0 {
		return sys.NewExitError(m.module.Name, uint32(closed>>32)) // Unpack the high order bits as the exit code.
	}
	return nil
}

// Name implements the same method as documented on api.Module
func (m *ModuleContext) Name() string {
	return m.module.Name
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (m *ModuleContext) WithMemory(memory *MemoryInstance) *ModuleContext {
	if memory != nil && memory != m.memory { // only re-allocate if it will change the effective memory
		return &ModuleContext{module: m.module, memory: memory, ctx: m.ctx, Sys: m.Sys, closed: m.closed}
	}
	return m
}

// String implements the same method as documented on api.Module
func (m *ModuleContext) String() string {
	return fmt.Sprintf("Module[%s]", m.Name())
}

// Context implements the same method as documented on api.Module
func (m *ModuleContext) Context() context.Context {
	return m.ctx
}

// WithContext implements the same method as documented on api.Module
func (m *ModuleContext) WithContext(ctx context.Context) api.Module {
	if ctx != nil && ctx != m.ctx { // only re-allocate if it will change the effective context
		return &ModuleContext{module: m.module, memory: m.memory, ctx: ctx, Sys: m.Sys, closed: m.closed}
	}
	return m
}

// Close implements the same method as documented on api.Module.
func (m *ModuleContext) Close() (err error) {
	return m.CloseWithExitCode(0)
}

// CloseWithExitCode implements the same method as documented on api.Module.
func (m *ModuleContext) CloseWithExitCode(exitCode uint32) (err error) {
	closed := uint64(1) + uint64(exitCode)<<32 // Store exitCode as high-order bits.
	if !atomic.CompareAndSwapUint64(m.closed, 0, closed) {
		return nil
	}
	m.module.Engine.Close()
	m.store.deleteModule(m.Name())
	if sys := m.Sys; sys != nil { // ex nil if from ModuleBuilder
		return sys.Close()
	}
	return
}

// Memory implements api.Module Memory
func (m *ModuleContext) Memory() api.Memory {
	return m.module.Memory
}

// ExportedMemory implements api.Module ExportedMemory
func (m *ModuleContext) ExportedMemory(name string) api.Memory {
	exp, err := m.module.getExport(name, ExternTypeMemory)
	if err != nil {
		return nil
	}
	return exp.Memory
}

// ExportedFunction implements api.Module ExportedFunction
func (m *ModuleContext) ExportedFunction(name string) api.Function {
	exp, err := m.module.getExport(name, ExternTypeFunc)
	if err != nil {
		return nil
	}
	if exp.Function.Module == m.module {
		return exp.Function
	} else {
		return &importedFn{importingModule: m, importedFn: exp.Function}
	}
}

// importedFn implements api.Function and ensures the call context of an imported function is the importing module.
type importedFn struct {
	importingModule *ModuleContext
	importedFn      *FunctionInstance
}

// ParamTypes implements the same method as documented on api.Function
func (f *importedFn) ParamTypes() []api.ValueType {
	return f.importedFn.ParamTypes()
}

// ResultTypes implements the same method as documented on api.Function
func (f *importedFn) ResultTypes() []api.ValueType {
	return f.importedFn.ResultTypes()
}

// Call implements the same method as documented on api.Function
func (f *importedFn) Call(m api.Module, params ...uint64) (ret []uint64, err error) {
	if m == nil {
		return f.importedFn.Call(f.importingModule, params...)
	}
	return f.importedFn.Call(m, params...)
}

// ParamTypes implements the same method as documented on api.Function
func (f *FunctionInstance) ParamTypes() []api.ValueType {
	return f.Type.Params
}

// ResultTypes implements the same method as documented on api.Function
func (f *FunctionInstance) ResultTypes() []api.ValueType {
	return f.Type.Results
}

// Call implements the same method as documented on api.Function
func (f *FunctionInstance) Call(m api.Module, params ...uint64) (ret []uint64, err error) {
	mod := f.Module
	modCtx, ok := m.(*ModuleContext)
	if ok {
		// TODO: check if the importing context is correct
	} else { // allow nil to substitute for the defining module
		modCtx = mod.Ctx
	}
	return mod.Engine.Call(modCtx, f, params...)
}

// ExportedGlobal implements api.Module ExportedGlobal
func (m *ModuleContext) ExportedGlobal(name string) api.Global {
	exp, err := m.module.getExport(name, ExternTypeGlobal)
	if err != nil {
		return nil
	}
	if exp.Global.Type.Mutable {
		return &mutableGlobal{exp.Global}
	}
	valType := exp.Global.Type.ValType
	switch valType {
	case ValueTypeI32:
		return globalI32(exp.Global.Val)
	case ValueTypeI64:
		return globalI64(exp.Global.Val)
	case ValueTypeF32:
		return globalF32(exp.Global.Val)
	case ValueTypeF64:
		return globalF64(exp.Global.Val)
	default:
		panic(fmt.Errorf("BUG: unknown value type %X", valType))
	}
}
