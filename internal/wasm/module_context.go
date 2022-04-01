package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// compile time check to ensure ModuleContext implements api.Module
var _ api.Module = &ModuleContext{}

func NewModuleContext(ctx context.Context, store *Store, instance *ModuleInstance, sys *SysContext) *ModuleContext {
	return &ModuleContext{ctx: ctx, memory: instance.Memory, module: instance, store: store, sys: sys}
}

// ModuleContext implements api.Module
type ModuleContext struct {
	// ctx is returned by Context and overridden WithContext
	ctx    context.Context
	module *ModuleInstance
	// memory is returned by Memory and overridden WithMemory
	memory api.Memory
	store  *Store

	// sys is not exposed publicly. This is currently only used by wasi.
	// Note: This is a part of ModuleContext so that scope is correct and Close is coherent.
	sys *SysContext
}

// Name implements the same method as documented on api.Module
func (m *ModuleContext) Name() string {
	return m.module.Name
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (m *ModuleContext) WithMemory(memory *MemoryInstance) *ModuleContext {
	if memory != nil && memory != m.memory { // only re-allocate if it will change the effective memory
		return &ModuleContext{module: m.module, memory: memory, ctx: m.ctx, sys: m.sys}
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

// Sys is exposed only for WASI.
func (m *ModuleContext) Sys() *SysContext {
	return m.sys
}

// WithContext implements the same method as documented on api.Module
func (m *ModuleContext) WithContext(ctx context.Context) api.Module {
	if ctx != nil && ctx != m.ctx { // only re-allocate if it will change the effective context
		return &ModuleContext{module: m.module, memory: m.memory, ctx: ctx, sys: m.sys}
	}
	return m
}

// Close implements the same method as documented on api.Module.
func (m *ModuleContext) Close() (err error) {
	return m.CloseWithExitCode(0)
}

// CloseWithExitCode implements the same method as documented on api.Module.
func (m *ModuleContext) CloseWithExitCode(exitCode uint32) (err error) {
	if err = m.store.CloseModuleWithExitCode(m.module.Name, exitCode); err != nil {
		return err
	} else if sys := m.sys; sys != nil { // ex nil if from ModuleBuilder
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
	return exp.Function
}

// ParamTypes implements api.Function ParamTypes
func (f *FunctionInstance) ParamTypes() []api.ValueType {
	return f.Type.Params
}

// ResultTypes implements api.Function ResultTypes
func (f *FunctionInstance) ResultTypes() []api.ValueType {
	return f.Type.Results
}

// Call implements api.Function Call
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
