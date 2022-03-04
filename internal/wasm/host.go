package internalwasm

import (
	"context"
	"fmt"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

// compile time check to ensure ModuleContext implements wasm.ModuleContext
var _ publicwasm.ModuleContext = &ModuleContext{}

func NewModuleContext(ctx context.Context, engine Engine, instance *ModuleInstance) *ModuleContext {
	return &ModuleContext{
		ctx:    ctx,
		engine: engine,
		memory: instance.MemoryInstance,
		Module: instance,
	}
}

// ModuleContext implements wasm.ModuleContext and wasm.Module
type ModuleContext struct {
	// ctx is returned by Context and overridden WithContext
	ctx context.Context
	// engine is used to implement function.Call
	engine Engine
	// Module is exported for spectests
	Module *ModuleInstance
	// memory is returned by Memory and overridden WithMemory
	memory publicwasm.Memory
}

// WithContext allows overriding context without re-allocation when the result would be the same.
func (c *ModuleContext) WithContext(ctx context.Context) *ModuleContext {
	// only re-allocate if it will change the effective context
	if ctx != nil && ctx != c.ctx {
		return &ModuleContext{engine: c.engine, Module: c.Module, memory: c.memory, ctx: ctx}
	}
	return c
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (c *ModuleContext) WithMemory(memory *MemoryInstance) *ModuleContext {
	// only re-allocate if it will change the effective memory
	if c.memory == nil || (memory != nil && memory.Max != nil && *memory.Max > 0 && memory != c.memory) {
		return &ModuleContext{engine: c.engine, Module: c.Module, memory: memory, ctx: c.ctx}
	}
	return c
}

// Context implements wasm.ModuleContext Context
func (c *ModuleContext) Context() context.Context {
	return c.ctx
}

// Memory implements wasm.ModuleContext Memory
func (c *ModuleContext) Memory() publicwasm.Memory {
	return c.memory
}

// Function implements wasm.ModuleContext Function
func (c *ModuleContext) Function(name string) publicwasm.Function {
	exp, err := c.Module.getExport(name, ExternTypeFunc)
	if err != nil {
		return nil
	}
	return &exportedFunction{module: c, function: exp.Function}
}

// exportedFunction wraps FunctionInstance so that it is called in context of the exporting module.
type exportedFunction struct {
	module   *ModuleContext
	function *FunctionInstance
}

// ParamTypes implements wasm.Function ParamTypes
func (f *exportedFunction) ParamTypes() []publicwasm.ValueType {
	return f.function.ParamTypes()
}

// ResultTypes implements wasm.Function ResultTypes
func (f *exportedFunction) ResultTypes() []publicwasm.ValueType {
	return f.function.ResultTypes()
}

// Call implements wasm.Function Call in the ModuleContext of the exporting module.
func (f *exportedFunction) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	modCtx := f.module.WithContext(ctx)
	return f.module.engine.Call(modCtx, f.function, params...)
}

// NewHostModule is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) NewHostModule(moduleName string, nameToGoFunc map[string]interface{}) (*HostModule, error) {
	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	exportCount := len(nameToGoFunc)
	ret := &HostModule{name: moduleName, NameToFunctionInstance: make(map[string]*FunctionInstance, exportCount)}
	hostModule := &ModuleInstance{
		Name:       moduleName,
		Exports:    make(map[string]*ExportInstance, exportCount),
		hostModule: ret,
	}

	for name, goFunc := range nameToGoFunc {
		hf, err := NewGoFunc(name, goFunc)
		if err != nil {
			return nil, err
		}

		f := &FunctionInstance{
			Name:           fmt.Sprintf("%s.%s", hostModule.Name, hf.wasmFunctionName),
			HostFunction:   hf.goFunc,
			FunctionKind:   hf.functionKind,
			ModuleInstance: hostModule,
		}
		hostModule.Exports[hf.wasmFunctionName] = &ExportInstance{Type: ExternTypeFunc, Function: f}
		hostModule.Functions = append(hostModule.Functions, f)
		ret.NameToFunctionInstance[name] = f

		if err = s.compileFunction(f, hf); err != nil {
			return nil, err
		}
	}

	s.moduleInstances[moduleName] = hostModule
	return ret, nil
}

func (s *Store) compileHostFunction(f *FunctionInstance, hf *GoFunc) (err error) {
	f.FunctionType, err = s.getTypeInstance(hf.functionType)
	if err != nil {
		return err
	}
	s.addFunctionInstances(f)

	if err = s.engine.Compile(f); err != nil {
		// On failure, we must release the function instance.
		if err = s.releaseFunctionInstances(f); err != nil {
			return err
		}
		return fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}
	return nil
}

func (s *Store) requireModuleUnused(moduleName string) error {
	if _, ok := s.moduleInstances[moduleName]; ok {
		return fmt.Errorf("module %s has already been instantiated", moduleName)
	}
	return nil
}

// HostModule implements wasm.HostModule
type HostModule struct {
	// name is for String and Store.ReleaseModuleInstance
	name                   string
	NameToFunctionInstance map[string]*FunctionInstance
}

// String implements fmt.Stringer
func (m *HostModule) String() string {
	return fmt.Sprintf("HostModule[%s]", m.name)
}

// ParamTypes implements wasm.HostFunction ParamTypes
func (f *FunctionInstance) ParamTypes() []publicwasm.ValueType {
	return f.FunctionType.Type.Params
}

// ResultTypes implements wasm.HostFunction ResultTypes
func (f *FunctionInstance) ResultTypes() []publicwasm.ValueType {
	return f.FunctionType.Type.Results
}

// Call implements wasm.HostFunction Call
func (f *FunctionInstance) Call(ctx publicwasm.ModuleContext, params ...uint64) ([]uint64, error) {
	modCtx, ok := ctx.(*ModuleContext)
	if !ok { // TODO: guard that modCtx.Module actually imported this!
		return nil, fmt.Errorf("this function was not imported by %s", ctx)
	}
	return modCtx.engine.Call(modCtx, f, params...)
}

// Function implements wasm.HostModule Function
func (m *HostModule) Function(name string) publicwasm.HostFunction {
	return m.NameToFunctionInstance[name]
}
