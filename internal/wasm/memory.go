package wasm

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	// MemoryPageSize is the unit of memory length in WebAssembly,
	// and is defined as 2^16 = 65536.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0
	MemoryPageSize = uint32(65536)
	// MemoryMaxPages is maximum number of pages defined (2^16).
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#grow-mem
	MemoryMaxPages = uint32(65536)
	// MemoryPageSizeInBits satisfies the relation: "1 << MemoryPageSizeInBits == MemoryPageSize".
	MemoryPageSizeInBits = 16
)

// MemoryInstance represents a memory instance in a store, and implements api.Memory.
//
// Note: In WebAssembly 1.0 (20191205), there may be up to one Memory per store, which means the precise memory is always
// wasm.Store Memories index zero: `store.Memories[0]`
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0.
type MemoryInstance struct {
	Buffer   []byte
	Min, Max uint32
}

// Size implements api.Memory Size
func (m *MemoryInstance) Size() uint32 {
	return uint32(len(m.Buffer))
}

// hasSize returns true if Len is sufficient for sizeInBytes at the given offset.
func (m *MemoryInstance) hasSize(offset uint32, sizeInBytes uint32) bool {
	return uint64(offset)+uint64(sizeInBytes) <= uint64(m.Size()) // uint64 prevents overflow on add
}

// ReadByte implements api.Memory ReadByte
func (m *MemoryInstance) ReadByte(offset uint32) (byte, bool) {
	if offset >= m.Size() {
		return 0, false
	}
	return m.Buffer[offset], true
}

// ReadUint32Le implements api.Memory ReadUint32Le
func (m *MemoryInstance) ReadUint32Le(offset uint32) (uint32, bool) {
	if !m.hasSize(offset, 4) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(m.Buffer[offset : offset+4]), true
}

// ReadFloat32Le implements api.Memory ReadFloat32Le
func (m *MemoryInstance) ReadFloat32Le(offset uint32) (float32, bool) {
	v, ok := m.ReadUint32Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float32frombits(v), true
}

// ReadUint64Le implements api.Memory ReadUint64Le
func (m *MemoryInstance) ReadUint64Le(offset uint32) (uint64, bool) {
	if !m.hasSize(offset, 8) {
		return 0, false
	}
	return binary.LittleEndian.Uint64(m.Buffer[offset : offset+8]), true
}

// ReadFloat64Le implements api.Memory ReadFloat64Le
func (m *MemoryInstance) ReadFloat64Le(offset uint32) (float64, bool) {
	v, ok := m.ReadUint64Le(offset)
	if !ok {
		return 0, false
	}
	return math.Float64frombits(v), true
}

// Read implements api.Memory Read
func (m *MemoryInstance) Read(offset, byteCount uint32) ([]byte, bool) {
	if !m.hasSize(offset, byteCount) {
		return nil, false
	}
	return m.Buffer[offset : offset+byteCount], true
}

// WriteByte implements api.Memory WriteByte
func (m *MemoryInstance) WriteByte(offset uint32, v byte) bool {
	if offset >= m.Size() {
		return false
	}
	m.Buffer[offset] = v
	return true
}

// WriteUint32Le implements api.Memory WriteUint32Le
func (m *MemoryInstance) WriteUint32Le(offset, v uint32) bool {
	if !m.hasSize(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint32(m.Buffer[offset:], v)
	return true
}

// WriteFloat32Le implements api.Memory WriteFloat32Le
func (m *MemoryInstance) WriteFloat32Le(offset uint32, v float32) bool {
	return m.WriteUint32Le(offset, math.Float32bits(v))
}

// WriteUint64Le implements api.Memory WriteUint64Le
func (m *MemoryInstance) WriteUint64Le(offset uint32, v uint64) bool {
	if !m.hasSize(offset, 8) {
		return false
	}
	binary.LittleEndian.PutUint64(m.Buffer[offset:], v)
	return true
}

// WriteFloat64Le implements api.Memory WriteFloat64Le
func (m *MemoryInstance) WriteFloat64Le(offset uint32, v float64) bool {
	return m.WriteUint64Le(offset, math.Float64bits(v))
}

// Write implements api.Memory Write
func (m *MemoryInstance) Write(offset uint32, val []byte) bool {
	if !m.hasSize(offset, uint32(len(val))) {
		return false
	}
	copy(m.Buffer[offset:], val)
	return true
}

// MemoryPagesToBytesNum converts the given pages into the number of bytes contained in these pages.
func MemoryPagesToBytesNum(pages uint32) (bytesNum uint64) {
	return uint64(pages) << MemoryPageSizeInBits
}

// memoryBytesNumToPages converts the given number of bytes into the number of pages.
func memoryBytesNumToPages(bytesNum uint64) (pages uint32) {
	return uint32(bytesNum >> MemoryPageSizeInBits)
}

// Grow extends the memory buffer by "newPages" * memoryPageSize.
// The logic here is described in https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#grow-mem.
//
// Returns -1 if the operation resulted in exceeding the maximum memory pages.
// Otherwise, returns the prior memory size after growing the memory buffer.
func (m *MemoryInstance) Grow(newPages uint32) (result uint32) {
	currentPages := memoryBytesNumToPages(uint64(len(m.Buffer)))

	// If exceeds the max of memory size, we push -1 according to the spec.
	if currentPages+newPages > m.Max {
		return 0xffffffff // = -1 in signed 32-bit integer.
	} else {
		// Otherwise, grow the memory.
		m.Buffer = append(m.Buffer, make([]byte, MemoryPagesToBytesNum(newPages))...)
		return currentPages
	}
}

// PageSize returns the current memory buffer size in pages.
func (m *MemoryInstance) PageSize() (result uint32) {
	return memoryBytesNumToPages(uint64(len(m.Buffer)))
}

// PagesToUnitOfBytes converts the pages to a human-readable form similar to what's specified. Ex. 1 -> "64Ki"
//
// See https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0
func PagesToUnitOfBytes(pages uint32) string {
	k := pages * 64
	if k < 1024 {
		return fmt.Sprintf("%d Ki", k)
	}
	m := k / 1024
	if m < 1024 {
		return fmt.Sprintf("%d Mi", m)
	}
	g := m / 1024
	if g < 1024 {
		return fmt.Sprintf("%d Gi", g)
	}
	return fmt.Sprintf("%d Ti", g/1024)
}
