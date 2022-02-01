package wasm

const (
	// memoryPageSize is the unit of memory length in WebAssembly,
	// and is defined as 2^16 = 65536.
	// https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0
	MemoryPageSize = 65536
	// The maximum number of pages is defined as 2^16.
	// https://www.w3.org/TR/wasm-core-1/#grow-mem
	memoryMaxPages = MemoryPageSize
	// memoryPageSizeInBit satisfies the relation: "1 << memoryPageSizeInBit == memoryPageSize".
	memoryPageSizeInBit = 16
)

// MemoryPagesToBytesNum converts the given pages into the number of bytes contained in these pages.
func memoryPagesToBytesNum(pages uint32) (bytesNum uint64) {
	return uint64(pages) << memoryPageSizeInBit
}

// MemoryPagesToBytesNum converts the given nuber of bytes into the number of pages.
func memoryBytesNumToPages(bytesNum uint64) (pages uint32) {
	return uint32(bytesNum >> memoryPageSizeInBit)
}
