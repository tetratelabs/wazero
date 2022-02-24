package internalwasm

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

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
	if memory != nil && memory.Max != nil && *memory.Max > 0 && memory != c.memory {
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
	exp, err := c.Module.GetExport(name, ExportKindFunc)
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

// ExportHostFunctions is defined internally for use in WASI tests and to keep the code size in the root directory small.
func (s *Store) ExportHostFunctions(moduleName string, nameToGoFunc map[string]interface{}) (publicwasm.HostExports, error) {
	if err := s.requireModuleUnused(moduleName); err != nil {
		return nil, err
	}

	exportCount := len(nameToGoFunc)

	hostModule := &ModuleInstance{Name: moduleName, Exports: make(map[string]*ExportInstance, exportCount)}
	s.ModuleInstances[moduleName] = hostModule
	ret := HostExports{NameToFunctionInstance: make(map[string]*FunctionInstance, exportCount)}
	for name, goFunc := range nameToGoFunc {
		if hf, err := NewGoFunc(name, goFunc); err != nil {
			return nil, err
		} else if function, err := s.AddHostFunction(hostModule, hf); err != nil {
			return nil, err
		} else {
			ret.NameToFunctionInstance[name] = function
		}
	}
	return &ret, nil
}

func (s *Store) requireModuleUnused(moduleName string) error {
	if _, ok := s.hostExports[moduleName]; ok {
		return fmt.Errorf("module %s has already been exported by this host", moduleName)
	}
	if _, ok := s.ModuleContexts[moduleName]; ok {
		return fmt.Errorf("module %s has already been instantiated", moduleName)
	}
	return nil
}

// HostExports implements wasm.HostExports
type HostExports struct {
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

// Function implements wasm.HostExports Function
func (g *HostExports) Function(name string) publicwasm.HostFunction {
	return g.NameToFunctionInstance[name]
}

// Size implements wasm.ModuleContext Size
func (m *MemoryInstance) Size() uint32 {
	return uint32(len(m.Buffer))
}

// hasSize returns true if Len is sufficient for sizeInBytes at the given offset.
func (m *MemoryInstance) hasSize(offset uint32, sizeInBytes uint32) bool {
	return uint64(offset+sizeInBytes) <= uint64(m.Size()) // uint64 prevents overflow on add
}

// ReadUint32Le implements wasm.ModuleContext ReadUint32Le
func (m *MemoryInstance) ReadUint32Le(offset uint32) (uint32, bool) {
	if !m.hasSize(offset, 4) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(m.Buffer[offset : offset+4]), true
}

// ReadFloat32Le implements wasm.ModuleContext ReadFloat32Le
func (m *MemoryInstance) ReadFloat32Le(offset uint32) (float32, bool) {
	v, ok := m.ReadUint32Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float32frombits(v), true
}

// ReadUint64Le implements wasm.ModuleContext ReadUint64Le
func (m *MemoryInstance) ReadUint64Le(offset uint32) (uint64, bool) {
	if !m.hasSize(offset, 8) {
		return 0, false
	}
	return binary.LittleEndian.Uint64(m.Buffer[offset : offset+8]), true
}

// ReadFloat64Le implements wasm.ModuleContext ReadFloat64Le
func (m *MemoryInstance) ReadFloat64Le(offset uint32) (float64, bool) {
	v, ok := m.ReadUint64Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float64frombits(v), true
}

// Read implements wasm.ModuleContext Read
func (m *MemoryInstance) Read(offset, byteCount uint32) ([]byte, bool) {
	if !m.hasSize(offset, byteCount) {
		return nil, false
	}
	return m.Buffer[offset : offset+byteCount], true
}

// WriteUint32Le implements wasm.ModuleContext WriteUint32Le
func (m *MemoryInstance) WriteUint32Le(offset, v uint32) bool {
	if !m.hasSize(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint32(m.Buffer[offset:], v)
	return true
}

// WriteFloat32Le implements wasm.ModuleContext WriteFloat32Le
func (m *MemoryInstance) WriteFloat32Le(offset uint32, v float32) bool {
	return m.WriteUint32Le(offset, math.Float32bits(v))
}

// WriteUint64Le implements wasm.ModuleContext WriteUint64Le
func (m *MemoryInstance) WriteUint64Le(offset uint32, v uint64) bool {
	if !m.hasSize(offset, 8) {
		return false
	}
	binary.LittleEndian.PutUint64(m.Buffer[offset:], v)
	return true
}

// WriteFloat64Le implements wasm.ModuleContext WriteFloat64Le
func (m *MemoryInstance) WriteFloat64Le(offset uint32, v float64) bool {
	return m.WriteUint64Le(offset, math.Float64bits(v))
}

// Write implements wasm.ModuleContext Write
func (m *MemoryInstance) Write(offset uint32, val []byte) bool {
	if !m.hasSize(offset, uint32(len(val))) {
		return false
	}
	copy(m.Buffer[offset:], val)
	return true
}

// NoopMemory is used when there is no memory or it has a max of zero pages.
var NoopMemory = &noopMemory{}

type noopMemory struct {
}

// Size implements wasm.ModuleContext Size
func (m *noopMemory) Size() uint32 {
	return 0
}

// ReadUint32Le implements wasm.ModuleContext ReadUint32Le
func (m *noopMemory) ReadUint32Le(_ uint32) (uint32, bool) {
	return 0, false
}

// ReadFloat32Le implements wasm.ModuleContext ReadFloat32Le
func (m *noopMemory) ReadFloat32Le(_ uint32) (float32, bool) {
	return 0, false
}

// ReadUint64Le implements wasm.ModuleContext ReadUint64Le
func (m *noopMemory) ReadUint64Le(_ uint32) (uint64, bool) {
	return 0, false
}

// ReadFloat64Le implements wasm.ModuleContext ReadFloat64Le
func (m *noopMemory) ReadFloat64Le(_ uint32) (float64, bool) {
	return 0, false
}

// Read implements wasm.ModuleContext Read
func (m *noopMemory) Read(_, _ uint32) ([]byte, bool) {
	return nil, false
}

// WriteUint32Le implements wasm.ModuleContext WriteUint32Le
func (m *noopMemory) WriteUint32Le(_, _ uint32) bool {
	return false
}

// WriteFloat32Le implements wasm.ModuleContext WriteFloat32Le
func (m *noopMemory) WriteFloat32Le(_ uint32, _ float32) bool {
	return false
}

// WriteUint64Le implements wasm.ModuleContext WriteUint64Le
func (m *noopMemory) WriteUint64Le(_ uint32, _ uint64) bool {
	return false
}

// WriteFloat64Le implements wasm.ModuleContext WriteFloat64Le
func (m *noopMemory) WriteFloat64Le(_ uint32, _ float64) bool {
	return false
}

// Write implements wasm.ModuleContext Write
func (m *noopMemory) Write(_ uint32, _ []byte) bool {
	return false
}
