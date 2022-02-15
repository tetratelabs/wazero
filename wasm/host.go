package wasm

import (
	"context"
	"encoding/binary"
	"math"
)

// hostFunctionCallContext is the first argument of all host functions.
type hostFunctionCallContext struct {
	ctx context.Context
	// Memory is the currently used memory instance at the time when the host function call is made.
	memory *MemoryInstance
	// TODO: Add others if necessary.
}

// NewHostFunctionCallContext creates a new api.HostFunctionCallContext with a context and memory instance.
func NewHostFunctionCallContext(ctx context.Context, memory *MemoryInstance) HostFunctionCallContext {
	return &hostFunctionCallContext{ctx: ctx, memory: memory}
}

// Context implements api.HostFunctionCallContext Context
func (c *hostFunctionCallContext) Context() context.Context {
	return c.ctx
}

// Memory implements api.HostFunctionCallContext Memory
func (c *hostFunctionCallContext) Memory() Memory {
	return c.memory
}

// Len implements api.HostFunctionCallContext Len
func (m *MemoryInstance) Len() uint32 {
	return uint32(len(m.Buffer))
}

// hasLen returns true if Len is sufficient for sizeInBytes at the given offset.
func (m *MemoryInstance) hasLen(offset uint32, sizeInBytes uint32) bool {
	return uint64(offset+sizeInBytes) <= uint64(m.Len()) // uint64 prevents overflow on add
}

// ReadUint32Le implements api.HostFunctionCallContext ReadUint32Le
func (m *MemoryInstance) ReadUint32Le(offset uint32) (uint32, bool) {
	if !m.hasLen(offset, 4) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(m.Buffer[offset : offset+4]), true
}

// ReadFloat32Le implements api.HostFunctionCallContext ReadFloat32Le
func (m *MemoryInstance) ReadFloat32Le(offset uint32) (float32, bool) {
	v, ok := m.ReadUint32Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float32frombits(v), true
}

// ReadUint64Le implements api.HostFunctionCallContext ReadUint64Le
func (m *MemoryInstance) ReadUint64Le(offset uint32) (uint64, bool) {
	if !m.hasLen(offset, 8) {
		return 0, false
	}
	return binary.LittleEndian.Uint64(m.Buffer[offset : offset+8]), true
}

// ReadFloat64Le implements api.HostFunctionCallContext ReadFloat64Le
func (m *MemoryInstance) ReadFloat64Le(offset uint32) (float64, bool) {
	v, ok := m.ReadUint64Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float64frombits(v), true
}

// Read implements api.HostFunctionCallContext Read
func (m *MemoryInstance) Read(offset, byteCount uint32) ([]byte, bool) {
	if !m.hasLen(offset, byteCount) {
		return nil, false
	}
	return m.Buffer[offset : offset+byteCount], true
}

// WriteUint32Le implements api.HostFunctionCallContext WriteUint32Le
func (m *MemoryInstance) WriteUint32Le(offset, v uint32) bool {
	if !m.hasLen(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint32(m.Buffer[offset:], v)
	return true
}

// WriteFloat32Le implements api.HostFunctionCallContext WriteFloat32Le
func (m *MemoryInstance) WriteFloat32Le(offset uint32, v float32) bool {
	return m.WriteUint32Le(offset, math.Float32bits(v))
}

// WriteUint64Le implements api.HostFunctionCallContext WriteUint64Le
func (m *MemoryInstance) WriteUint64Le(offset uint32, v uint64) bool {
	if !m.hasLen(offset, 8) {
		return false
	}
	binary.LittleEndian.PutUint64(m.Buffer[offset:], v)
	return true
}

// WriteFloat64Le implements api.HostFunctionCallContext WriteFloat64Le
func (m *MemoryInstance) WriteFloat64Le(offset uint32, v float64) bool {
	return m.WriteUint64Le(offset, math.Float64bits(v))
}

// Write implements api.HostFunctionCallContext Write
func (m *MemoryInstance) Write(offset uint32, val []byte) bool {
	if !m.hasLen(offset, uint32(len(val))) {
		return false
	}
	copy(m.Buffer[offset:], val)
	return true
}
