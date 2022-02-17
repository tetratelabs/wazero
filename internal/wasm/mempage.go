package internalwasm

const (
	// MemoryPageSize is the unit of memory length in WebAssembly,
	// and is defined as 2^16 = 65536.
	// See https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0
	MemoryPageSize = uint32(65536)
	// MemoryMaxPages is maximum number of pages defined (2^16).
	// See https://www.w3.org/TR/wasm-core-1/#grow-mem
	MemoryMaxPages = MemoryPageSize
	// MemoryPageSizeInBits satisfies the relation: "1 << MemoryPageSizeInBits == MemoryPageSize".
	MemoryPageSizeInBits = 16
)

// MemoryPagesToBytesNum converts the given pages into the number of bytes contained in these pages.
func memoryPagesToBytesNum(pages uint32) (bytesNum uint64) {
	return uint64(pages) << MemoryPageSizeInBits
}

// MemoryPagesToBytesNum converts the given number of bytes into the number of pages.
func memoryBytesNumToPages(bytesNum uint64) (pages uint32) {
	return uint32(bytesNum >> MemoryPageSizeInBits)
}

// Grow extends the memory buffer by "newPages" * memoryPageSize.
// The logic here is described in https://www.w3.org/TR/wasm-core-1/#grow-mem.
//
// Returns -1 if the operation resulted in excedding the maximum memory pages.
// Otherwise, returns the prior memory size after growing the memory buffer.
func (m *MemoryInstance) Grow(newPages uint32) (result uint32) {
	currentPages := memoryBytesNumToPages(uint64(len(m.Buffer)))

	maxPages := uint32(MemoryMaxPages)
	if m.Max != nil {
		maxPages = *m.Max
	}

	// If exceeds the max of memory size, we push -1 according to the spec.
	if currentPages+newPages > maxPages {
		return 0xffffffff // = -1 in signed 32 bit integer.
	} else {
		// Otherwise, grow the memory.
		m.Buffer = append(m.Buffer, make([]byte, memoryPagesToBytesNum(newPages))...)
		return currentPages
	}
}

// PageSize returns the current memory buffer size in pages.
func (m *MemoryInstance) PageSize() (result uint32) {
	return memoryBytesNumToPages(uint64(len(m.Buffer)))
}
