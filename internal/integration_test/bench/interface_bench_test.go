package bench

import (
	"encoding/binary"
	"testing"
)

func BenchmarkInterfaceVSUnexport(b *testing.B) {
	const size = 100
	const val = 55
	var memInterface MemoryInstanceInterface = &memoryInstance{buffer: make([]byte, size)}
	memStruct := &memoryInstance{buffer: make([]byte, size)}

	b.Run("interface", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for addr := uint32(0); addr < size+1000; addr++ {
				memInterface.PutUint32(addr, val)
			}
		}
	})
	b.Run("struct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for addr := uint32(0); addr < size+1000; addr++ {
				memStruct.PutUint32(addr, val)
			}
		}
	})
}

type MemoryInstanceInterface interface {
	PutUint32(addr uint32, val uint32) bool
}

type memoryInstance struct {
	buffer []byte // inexpert only this
	Min    uint32 // PS especially below would make sense to also not export as export makes it mutable
	Max    *uint32
}

func (m *memoryInstance) validateAddrRange(addr uint32, rangeSize uint64) bool {
	return uint64(addr) < uint64(len(m.buffer)) && rangeSize <= uint64(len(m.buffer))-uint64(addr)
}

func (m *memoryInstance) PutUint32(addr uint32, val uint32) bool {
	if !m.validateAddrRange(addr, uint64(4)) {
		return false
	}
	binary.LittleEndian.PutUint32(m.buffer[addr:], val)
	return true
}
