//go:build !tinygo

package wasm

func lengthMemoryPages(newPage uint32) int {
	return int(MemoryPagesToBytesNum(newPage))
}
