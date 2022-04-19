package wasm

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

// compile time check to ensure CallContext implements api.Module
var _ api.Module = &CallContext{}

func NewCallContext(store *Store, instance *ModuleInstance, Sys *SysContext) *CallContext {
	zero := uint64(0)
	return &CallContext{memory: instance.Memory, module: instance, store: store, Sys: Sys, closed: &zero}
}

// CallContext is a function call context bound to a module. This is important as one module's functions can call
// imported functions, but all need to effect the same memory.
//
// Note: This does not include the context.Context because doing so risks caching the wrong context which can break
// functionality like trace propagation.
// Note: this also implements api.Module in order to simplify usage as a host function parameter.
type CallContext struct {
	module *ModuleInstance
	// memory is returned by Memory and overridden WithMemory
	memory api.Memory
	store  *Store

	// Note: This is a part of CallContext so that scope is correct and Close is coherent.
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
func (m *CallContext) FailIfClosed() error {
	if closed := atomic.LoadUint64(m.closed); closed != 0 {
		return sys.NewExitError(m.module.Name, uint32(closed>>32)) // Unpack the high order bits as the exit code.
	}
	return nil
}

// Name implements the same method as documented on api.Module
func (m *CallContext) Name() string {
	return m.module.Name
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (m *CallContext) WithMemory(memory *MemoryInstance) *CallContext {
	if memory != nil && memory != m.memory { // only re-allocate if it will change the effective memory
		return &CallContext{module: m.module, memory: memory, Sys: m.Sys, closed: m.closed}
	}
	return m
}

// String implements the same method as documented on api.Module
func (m *CallContext) String() string {
	return fmt.Sprintf("Module[%s]", m.Name())
}

// Close implements the same method as documented on api.Module.
func (m *CallContext) Close() (err error) {
	return m.CloseWithExitCode(0)
}

// CloseWithExitCode implements the same method as documented on api.Module.
func (m *CallContext) CloseWithExitCode(exitCode uint32) (err error) {
	closed := uint64(1) + uint64(exitCode)<<32 // Store exitCode as high-order bits.
	if !atomic.CompareAndSwapUint64(m.closed, 0, closed) {
		return nil
	}
	m.store.deleteModule(m.Name())
	if sys := m.Sys; sys != nil { // ex nil if from ModuleBuilder
		return sys.Close()
	}
	return
}

// Memory implements api.Module Memory
func (m *CallContext) Memory() api.Memory {
	return m.module.Memory
}

// ExportedMemory implements api.Module ExportedMemory
func (m *CallContext) ExportedMemory(name string) api.Memory {
	exp, err := m.module.getExport(name, ExternTypeMemory)
	if err != nil {
		return nil
	}
	return exp.Memory
}

// ExportedFunction implements api.Module ExportedFunction
func (m *CallContext) ExportedFunction(name string) api.Function {
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
	importingModule *CallContext
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
func (f *importedFn) Call(ctx context.Context, params ...uint64) (ret []uint64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	mod := f.importingModule
	return f.importedFn.Module.Engine.Call(ctx, mod, f.importedFn, params...)
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
func (f *FunctionInstance) Call(ctx context.Context, params ...uint64) (ret []uint64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	mod := f.Module
	return mod.Engine.Call(ctx, mod.CallCtx, f, params...)
}

// ExportedGlobal implements api.Module ExportedGlobal
func (m *CallContext) ExportedGlobal(name string) api.Global {
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
