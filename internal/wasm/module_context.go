package internalwasm

import (
	"context"
	"fmt"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// compile time check to ensure ModuleContext implements wasm.Module
var _ publicwasm.Module = &ModuleContext{}

func NewModuleContext(ctx context.Context, store *Store, instance *ModuleInstance) *ModuleContext {
	return &ModuleContext{
		ctx:    ctx,
		memory: instance.Memory,
		module: instance,
		store:  store,
	}
}

// ModuleContext implements wasm.Module
type ModuleContext struct {
	// ctx is returned by Context and overridden WithContext
	ctx    context.Context
	module *ModuleInstance
	// memory is returned by Memory and overridden WithMemory
	memory publicwasm.Memory
	store  *Store
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (m *ModuleContext) WithMemory(memory *MemoryInstance) *ModuleContext {
	// only re-allocate if it will change the effective memory
	if m.memory == nil || (memory != nil && memory.Max != nil && *memory.Max > 0 && memory != m.memory) {
		return &ModuleContext{module: m.module, memory: memory, ctx: m.ctx}
	}
	return m
}

// String implements fmt.Stringer
func (m *ModuleContext) String() string {
	return fmt.Sprintf("Module[%s]", m.module.Name)
}

// Context implements wasm.Module Context
func (m *ModuleContext) Context() context.Context {
	return m.ctx
}

// WithContext implements wasm.Module WithContext
func (m *ModuleContext) WithContext(ctx context.Context) publicwasm.Module {
	// only re-allocate if it will change the effective context
	if ctx != nil && ctx != m.ctx {
		return &ModuleContext{module: m.module, memory: m.memory, ctx: ctx}
	}
	return m
}

// Close implements io.Closer
func (m *ModuleContext) Close() error {
	return m.store.CloseModule(m.module.Name)
}

// Memory implements wasm.Module Memory
func (m *ModuleContext) Memory() publicwasm.Memory {
	return m.module.Memory
}

// ExportedMemory implements wasm.Module ExportedMemory
func (m *ModuleContext) ExportedMemory(name string) publicwasm.Memory {
	exp, err := m.module.getExport(name, ExternTypeMemory)
	if err != nil {
		return nil
	}
	return exp.Memory
}

// ExportedFunction implements wasm.Module ExportedFunction
func (m *ModuleContext) ExportedFunction(name string) publicwasm.Function {
	exp, err := m.module.getExport(name, ExternTypeFunc)
	if err != nil {
		return nil
	}
	return exp.Function
}

// ParamTypes implements wasm.Function ParamTypes
func (f *FunctionInstance) ParamTypes() []publicwasm.ValueType {
	return f.Type.Params
}

// ResultTypes implements wasm.Function ResultTypes
func (f *FunctionInstance) ResultTypes() []publicwasm.ValueType {
	return f.Type.Results
}

// Call implements wasm.Function Call
func (f *FunctionInstance) Call(ctx publicwasm.Module, params ...uint64) ([]uint64, error) {
	mod := f.Module
	if modCtx, ok := ctx.(*ModuleContext); !ok { // allow nil to substitute for the defining module
		return mod.Engine.Call(mod.Ctx, f, params...)
	} else { // TODO: check if the importing context is correct
		return mod.Engine.Call(modCtx, f, params...)
	}
}

// ExportedGlobal implements wasm.Module ExportedGlobal
func (m *ModuleContext) ExportedGlobal(name string) publicwasm.Global {
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
