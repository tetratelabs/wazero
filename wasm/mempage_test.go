package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryPageSizeConsts(t *testing.T) {
	require.Equal(t, MemoryPageSize, 1<<memoryPageSizeInBit)
}

func Test_MemoryPagesToBytesNum(t *testing.T) {
	for _, numPage := range []uint32{0, 1, 5, 10} {
		require.Equal(t, uint64(numPage)*MemoryPageSize, memoryPagesToBytesNum(numPage))
	}
}

func Test_MemoryBytesNumToPages(t *testing.T) {
	for _, numbytes := range []uint64{0, MemoryPageSize * 1, MemoryPageSize * 10} {
		require.Equal(t, uint32(numbytes/MemoryPageSize), memoryBytesNumToPages(numbytes))
	}
}
