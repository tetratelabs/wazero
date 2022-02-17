package internalwasm

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

type HostFunction struct {
	name         string
	functionType *FunctionType
	goFunc       *reflect.Value
}

// GetFunctionType returns the function type corresponding to the function signature or errs if invalid.
func GetFunctionType(name string, fn *reflect.Value) (*FunctionType, error) {
	if fn.Kind() != reflect.Func {
		return nil, fmt.Errorf("%s value is not a reflect.Func: %s", name, fn.String())
	}
	p := fn.Type()
	if p.NumIn() == 0 { // TODO: actually check the type
		return nil, fmt.Errorf("%s must accept wasm.HostFunctionCallContext as the first param", name)
	}

	paramTypes := make([]ValueType, p.NumIn()-1)
	for i := range paramTypes {
		kind := p.In(i + 1).Kind()
		if t, ok := getTypeOf(kind); !ok {
			return nil, fmt.Errorf("%s param[%d] is unsupported: %s", name, i, kind.String())
		} else {
			paramTypes[i] = t
		}
	}

	resultTypes := make([]ValueType, p.NumOut())
	for i := range resultTypes {
		kind := p.Out(i).Kind()
		if t, ok := getTypeOf(kind); !ok {
			return nil, fmt.Errorf("%s result[%d] is unsupported: %s", name, i, kind.String())
		} else {
			resultTypes[i] = t
		}
	}
	return &FunctionType{Params: paramTypes, Results: resultTypes}, nil
}

func getTypeOf(kind reflect.Kind) (ValueType, bool) {
	switch kind {
	case reflect.Float64:
		return ValueTypeF64, true
	case reflect.Float32:
		return ValueTypeF32, true
	case reflect.Int32, reflect.Uint32:
		return ValueTypeI32, true
	case reflect.Int64, reflect.Uint64:
		return ValueTypeI64, true
	default:
		return 0x00, false
	}
}

// compile time check to ensure HostFunctionCallContext implements publicwasm.HostFunctionCallContext
var _ publicwasm.HostFunctionCallContext = &HostFunctionCallContext{}

func NewHostFunctionCallContext(s *Store, instance *ModuleInstance) *HostFunctionCallContext {
	return &HostFunctionCallContext{
		ctx:    context.Background(),
		memory: instance.Memory,
		module: instance,
		store:  s,
	}
}

// HostFunctionCallContext implements wasm.HostFunctionCallContext and wasm.Module
type HostFunctionCallContext struct {
	// ctx is exposed as wasm.HostFunctionCallContext Context
	ctx context.Context
	// memory is exposed as wasm.HostFunctionCallContext Memory
	memory publicwasm.Memory
	// module is the current module
	module *ModuleInstance
	// store is a reference to Store.Engine and Store.HostFunctionCallContexts
	store *Store
}

// WithContext allows overriding context without re-allocation when the result would be the same.
func (c *HostFunctionCallContext) WithContext(ctx context.Context) *HostFunctionCallContext {
	// only re-allocate if it will change the effective context
	if ctx != nil && ctx != c.ctx {
		return &HostFunctionCallContext{ctx: ctx, memory: c.memory, module: c.module, store: c.store}
	}
	return c
}

// WithMemory allows overriding memory without re-allocation when the result would be the same.
func (c *HostFunctionCallContext) WithMemory(memory *MemoryInstance) *HostFunctionCallContext {
	// only re-allocate if it will change the effective memory
	if memory != nil && memory.Max != nil && *memory.Max > 0 && memory != c.memory {
		return &HostFunctionCallContext{ctx: c.ctx, memory: memory, module: c.module, store: c.store}
	}
	return c
}

func (c *HostFunctionCallContext) Context() context.Context {
	return c.ctx
}

// Memory implements wasm.Module Memory
func (c *HostFunctionCallContext) Memory() publicwasm.Memory {
	return c.memory
}

// Functions implements wasm.HostFunctionCallContext Functions
func (c *HostFunctionCallContext) Functions() publicwasm.ModuleFunctions {
	return c
}

// FunctionsForModule implements wasm.HostFunctionCallContext FunctionsForModule
func (c *HostFunctionCallContext) FunctionsForModule(moduleName string) (publicwasm.ModuleFunctions, bool) {
	c, ok := c.store.HostFunctionCallContexts[moduleName]
	if !ok {
		return nil, false
	}
	return c, true
}

func (c *HostFunctionCallContext) GetFunctionVoidReturn(name string) (publicwasm.FunctionVoidReturn, bool) {
	f, err := c.module.GetFunctionVoidReturn(name)
	if err != nil {
		return nil, false
	}
	return (&functionVoidReturn{c: c, f: f}).Call, true
}

func (c *HostFunctionCallContext) GetFunctionI32Return(name string) (publicwasm.FunctionI32Return, bool) {
	f, err := c.module.GetFunction(name, ValueTypeI32)
	if err != nil {
		return nil, false
	}
	return (&functionI32Return{c: c, f: f}).Call, true
}

func (c *HostFunctionCallContext) GetFunctionI64Return(name string) (publicwasm.FunctionI64Return, bool) {
	f, err := c.module.GetFunction(name, ValueTypeI64)
	if err != nil {
		return nil, false
	}
	return (&functionI64Return{c: c, f: f}).Call, true
}

func (c *HostFunctionCallContext) GetFunctionF32Return(name string) (publicwasm.FunctionF32Return, bool) {
	f, err := c.module.GetFunction(name, ValueTypeF32)
	if err != nil {
		return nil, false
	}
	return (&functionF32Return{c: c, f: f}).Call, true
}

func (c *HostFunctionCallContext) GetFunctionF64Return(name string) (publicwasm.FunctionF64Return, bool) {
	f, err := c.module.GetFunction(name, ValueTypeF64)
	if err != nil {
		return nil, false
	}
	return (&functionF64Return{c: c, f: f}).Call, true
}

type functionVoidReturn struct {
	c *HostFunctionCallContext
	f *FunctionInstance
}

func (f *functionVoidReturn) Call(ctx context.Context, params ...uint64) error {
	_, err := call(f.c.WithContext(ctx), f.f, params...)
	return err
}

type functionI32Return struct {
	c *HostFunctionCallContext
	f *FunctionInstance
}

func (f *functionI32Return) Call(ctx context.Context, params ...uint64) (uint32, error) {
	results, err := call(f.c.WithContext(ctx), f.f, params...)
	if err != nil {
		return 0, err
	}
	return uint32(results[0]), nil
}

type functionI64Return struct {
	c *HostFunctionCallContext
	f *FunctionInstance
}

func (f *functionI64Return) Call(ctx context.Context, params ...uint64) (uint64, error) {
	results, err := call(f.c.WithContext(ctx), f.f, params...)
	if err != nil {
		return 0, err
	}
	return results[0], nil
}

type functionF32Return struct {
	c *HostFunctionCallContext
	f *FunctionInstance
}

func (f *functionF32Return) Call(ctx context.Context, params ...uint64) (float32, error) {
	results, err := call(f.c.WithContext(ctx), f.f, params...)
	if err != nil {
		return 0, err
	}
	return float32(math.Float64frombits(results[0])), nil
}

type functionF64Return struct {
	c *HostFunctionCallContext
	f *FunctionInstance
}

func (f *functionF64Return) Call(ctx context.Context, params ...uint64) (float64, error) {
	results, err := call(f.c.WithContext(ctx), f.f, params...)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(results[0]), nil
}

func call(ctx *HostFunctionCallContext, f *FunctionInstance, params ...uint64) ([]uint64, error) {
	paramSignature := f.FunctionType.Type.Params
	paramCount := len(params)
	if len(paramSignature) != paramCount {
		return nil, fmt.Errorf("expected %d params, but passed %d", len(paramSignature), paramCount)
	}
	return ctx.store.Engine.Call(ctx, f, params...)
}

// Len implements wasm.HostFunctionCallContext Len
func (m *MemoryInstance) Len() uint32 {
	return uint32(len(m.Buffer))
}

// hasLen returns true if Len is sufficient for sizeInBytes at the given offset.
func (m *MemoryInstance) hasLen(offset uint32, sizeInBytes uint32) bool {
	return uint64(offset+sizeInBytes) <= uint64(m.Len()) // uint64 prevents overflow on add
}

// ReadUint32Le implements wasm.HostFunctionCallContext ReadUint32Le
func (m *MemoryInstance) ReadUint32Le(offset uint32) (uint32, bool) {
	if !m.hasLen(offset, 4) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(m.Buffer[offset : offset+4]), true
}

// ReadFloat32Le implements wasm.HostFunctionCallContext ReadFloat32Le
func (m *MemoryInstance) ReadFloat32Le(offset uint32) (float32, bool) {
	v, ok := m.ReadUint32Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float32frombits(v), true
}

// ReadUint64Le implements wasm.HostFunctionCallContext ReadUint64Le
func (m *MemoryInstance) ReadUint64Le(offset uint32) (uint64, bool) {
	if !m.hasLen(offset, 8) {
		return 0, false
	}
	return binary.LittleEndian.Uint64(m.Buffer[offset : offset+8]), true
}

// ReadFloat64Le implements wasm.HostFunctionCallContext ReadFloat64Le
func (m *MemoryInstance) ReadFloat64Le(offset uint32) (float64, bool) {
	v, ok := m.ReadUint64Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float64frombits(v), true
}

// Read implements wasm.HostFunctionCallContext Read
func (m *MemoryInstance) Read(offset, byteCount uint32) ([]byte, bool) {
	if !m.hasLen(offset, byteCount) {
		return nil, false
	}
	return m.Buffer[offset : offset+byteCount], true
}

// WriteUint32Le implements wasm.HostFunctionCallContext WriteUint32Le
func (m *MemoryInstance) WriteUint32Le(offset, v uint32) bool {
	if !m.hasLen(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint32(m.Buffer[offset:], v)
	return true
}

// WriteFloat32Le implements wasm.HostFunctionCallContext WriteFloat32Le
func (m *MemoryInstance) WriteFloat32Le(offset uint32, v float32) bool {
	return m.WriteUint32Le(offset, math.Float32bits(v))
}

// WriteUint64Le implements wasm.HostFunctionCallContext WriteUint64Le
func (m *MemoryInstance) WriteUint64Le(offset uint32, v uint64) bool {
	if !m.hasLen(offset, 8) {
		return false
	}
	binary.LittleEndian.PutUint64(m.Buffer[offset:], v)
	return true
}

// WriteFloat64Le implements wasm.HostFunctionCallContext WriteFloat64Le
func (m *MemoryInstance) WriteFloat64Le(offset uint32, v float64) bool {
	return m.WriteUint64Le(offset, math.Float64bits(v))
}

// Write implements wasm.HostFunctionCallContext Write
func (m *MemoryInstance) Write(offset uint32, val []byte) bool {
	if !m.hasLen(offset, uint32(len(val))) {
		return false
	}
	copy(m.Buffer[offset:], val)
	return true
}

// NoopMemory is used when there is no memory or it has a max of zero pages.
var NoopMemory = &noopMemory{}

type noopMemory struct {
}

// Len implements wasm.HostFunctionCallContext Len
func (m *noopMemory) Len() uint32 {
	return 0
}

// ReadUint32Le implements wasm.HostFunctionCallContext ReadUint32Le
func (m *noopMemory) ReadUint32Le(_ uint32) (uint32, bool) {
	return 0, false
}

// ReadFloat32Le implements wasm.HostFunctionCallContext ReadFloat32Le
func (m *noopMemory) ReadFloat32Le(_ uint32) (float32, bool) {
	return 0, false
}

// ReadUint64Le implements wasm.HostFunctionCallContext ReadUint64Le
func (m *noopMemory) ReadUint64Le(_ uint32) (uint64, bool) {
	return 0, false
}

// ReadFloat64Le implements wasm.HostFunctionCallContext ReadFloat64Le
func (m *noopMemory) ReadFloat64Le(_ uint32) (float64, bool) {
	return 0, false
}

// Read implements wasm.HostFunctionCallContext Read
func (m *noopMemory) Read(_, _ uint32) ([]byte, bool) {
	return nil, false
}

// WriteUint32Le implements wasm.HostFunctionCallContext WriteUint32Le
func (m *noopMemory) WriteUint32Le(_, _ uint32) bool {
	return false
}

// WriteFloat32Le implements wasm.HostFunctionCallContext WriteFloat32Le
func (m *noopMemory) WriteFloat32Le(_ uint32, _ float32) bool {
	return false
}

// WriteUint64Le implements wasm.HostFunctionCallContext WriteUint64Le
func (m *noopMemory) WriteUint64Le(_ uint32, _ uint64) bool {
	return false
}

// WriteFloat64Le implements wasm.HostFunctionCallContext WriteFloat64Le
func (m *noopMemory) WriteFloat64Le(_ uint32, _ float64) bool {
	return false
}

// Write implements wasm.HostFunctionCallContext Write
func (m *noopMemory) Write(_ uint32, _ []byte) bool {
	return false
}
