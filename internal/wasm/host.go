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
	exp, err := c.Module.GetExport(name, ExternTypeFunc)
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
func (s *Store) NewHostModule(moduleName string, nameToGoFunc map[string]interface{}) (publicwasm.HostModule, error) {
	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	exportCount := len(nameToGoFunc)

	ret := &HostModule{NameToFunctionInstance: make(map[string]*FunctionInstance, exportCount)}
	hostModule := &ModuleInstance{
		Name: moduleName, Exports: make(map[string]*ExportInstance, exportCount),
		hostModule: ret,
	}
	s.moduleInstances[moduleName] = hostModule
	for name, goFunc := range nameToGoFunc {
		if hf, err := NewGoFunc(name, goFunc); err != nil {
			return nil, err
		} else if function, err := s.AddHostFunction(hostModule, hf); err != nil {
			return nil, err
		} else {
			ret.NameToFunctionInstance[name] = function
		}
	}
	return ret, nil
}

// AddHostFunction exports a function so that it can be imported under the given module and name. If a function already
// exists for this module and name it is ignored rather than overwritten.
//
// Note: The wasm.Memory of the fn will be from the importing module.
func (s *Store) AddHostFunction(m *ModuleInstance, hf *GoFunc) (*FunctionInstance, error) {
	typeInstance, err := s.getTypeInstance(hf.functionType)
	if err != nil {
		return nil, err
	}

	f := &FunctionInstance{
		Name:           fmt.Sprintf("%s.%s", m.Name, hf.wasmFunctionName),
		HostFunction:   hf.goFunc,
		FunctionKind:   hf.functionKind,
		FunctionType:   typeInstance,
		ModuleInstance: m,
	}

	s.addFunctionInstances(f)

	if err = s.engine.Compile(f); err != nil {
		// On failure, we must release the function instance.
		if err := s.releaseFunctionInstances(f); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}

	if err = m.addExport(hf.wasmFunctionName, &ExportInstance{Type: ExternTypeFunc, Function: f}); err != nil {
		// On failure, we must release the function instance.
		if err := s.releaseFunctionInstances(f); err != nil {
			return nil, err
		}
		return nil, err
	}

	m.Functions = append(m.Functions, f)
	return f, nil
}

// Only used in spectest.
func (s *Store) AddHostGlobal(m *ModuleInstance, name string, value uint64, valueType ValueType, mutable bool) error {
	if mutable {
		if err := s.EnabledFeatures.Require(FeatureMutableGlobal); err != nil {
			return err
		}
	}
	g := &GlobalInstance{
		Val:  value,
		Type: &GlobalType{Mutable: mutable, ValType: valueType},
	}

	m.Globals = append(m.Globals, g)
	s.addGlobalInstances(g)

	return m.addExport(name, &ExportInstance{Type: ExternTypeGlobal, Global: g})
}

// Only used in spectest as of now.
func (s *Store) AddHostTableInstance(m *ModuleInstance, name string, min uint32, max *uint32) error {
	t := newTableInstance(min, max)

	// TODO: check if the module already has memory, and if so, returns error.
	m.TableInstance = t
	s.addTableInstance(t)

	return m.addExport(name, &ExportInstance{Type: ExternTypeTable, Table: t})
}

// Only used in spectest as of now.
func (s *Store) AddHostMemoryInstance(m *ModuleInstance, name string, min uint32, max *uint32) error {
	memory := &MemoryInstance{
		Buffer: make([]byte, MemoryPagesToBytesNum(min)),
		Min:    min,
		Max:    max,
	}

	// TODO: check if the module already has memory, and if so, returns error.
	m.MemoryInstance = memory
	s.addMemoryInstance(memory)

	return m.addExport(name, &ExportInstance{Type: ExternTypeMemory, Memory: memory})
}

func (s *Store) requireModuleUnused(moduleName string) error {
	if _, ok := s.moduleInstances[moduleName]; ok {
		return fmt.Errorf("module %s has already been instantiated", moduleName)
	}
	return nil
}

// HostModule implements wasm.HostModule
type HostModule struct {
	NameToFunctionInstance map[string]*FunctionInstance
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
func (g *HostModule) Function(name string) publicwasm.HostFunction {
	return g.NameToFunctionInstance[name]
}
