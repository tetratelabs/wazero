//go:build tinygo

package wasm

func lengthMemoryPages(newPage uint32) uintptr {
	return uintptr(MemoryPagesToBytesNum(newPage))
}
