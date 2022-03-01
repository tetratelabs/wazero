package internalwasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryPageConsts(t *testing.T) {
	require.Equal(t, MemoryPageSize, uint32(1)<<MemoryPageSizeInBits)
	require.Equal(t, MemoryPageSize, MemoryMaxPages)
	require.Equal(t, MemoryPageSize, uint32(1<<16))
}

func Test_MemoryPagesToBytesNum(t *testing.T) {
	for _, numPage := range []uint32{0, 1, 5, 10} {
		require.Equal(t, uint64(numPage*MemoryPageSize), MemoryPagesToBytesNum(numPage))
	}
}

func Test_MemoryBytesNumToPages(t *testing.T) {
	for _, numbytes := range []uint32{0, MemoryPageSize * 1, MemoryPageSize * 10} {
		require.Equal(t, numbytes/MemoryPageSize, memoryBytesNumToPages(uint64(numbytes)))
	}
}

func TestMemoryInstance_Grow_Size(t *testing.T) {
	t.Run("with max", func(t *testing.T) {
		max := uint32(10)
		m := &MemoryInstance{Max: &max, Buffer: make([]byte, 0)}
		require.Equal(t, uint32(0), m.Grow(5))
		require.Equal(t, uint32(5), m.PageSize())
		// Zero page grow is well-defined, should return the current page correctly.
		require.Equal(t, uint32(5), m.Grow(0))
		require.Equal(t, uint32(5), m.PageSize())
		require.Equal(t, uint32(5), m.Grow(4))
		require.Equal(t, uint32(9), m.PageSize())
		// At this point, the page size equal 9,
		// so trying to grow two pages should result in failure.
		require.Equal(t, int32(-1), int32(m.Grow(2)))
		require.Equal(t, uint32(9), m.PageSize())
		// But growing one page is still perimitted.
		require.Equal(t, uint32(9), m.Grow(1))
		// Ensure that the current page size equals the max.
		require.Equal(t, max, m.PageSize())
	})
	t.Run("without max", func(t *testing.T) {
		m := &MemoryInstance{Buffer: make([]byte, 0)}
		require.Equal(t, uint32(0), m.Grow(1))
		require.Equal(t, uint32(1), m.PageSize())
		// Trying to grow above MemoryMaxPages, the operation should fail.
		require.Equal(t, int32(-1), int32(m.Grow(MemoryMaxPages)))
		require.Equal(t, uint32(1), m.PageSize())
	})
}

func TestReadByte(t *testing.T) {
	var mem = &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 0, 0, 0, 16}, Min: 1}
	v, ok := mem.ReadByte(7)
	require.True(t, ok)
	require.Equal(t, byte(16), v)

	_, ok = mem.ReadByte(8)
	require.False(t, ok)

	_, ok = mem.ReadByte(9)
	require.False(t, ok)
}

func TestReadUint32Le(t *testing.T) {
	var mem = &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1}
	v, ok := mem.ReadUint32Le(4)
	require.True(t, ok)
	require.Equal(t, uint32(16), v)

	_, ok = mem.ReadUint32Le(5)
	require.False(t, ok)

	_, ok = mem.ReadUint32Le(9)
	require.False(t, ok)
}

func TestWriteUint32Le(t *testing.T) {
	var mem = &MemoryInstance{Buffer: make([]byte, 8), Min: 1}
	require.True(t, mem.WriteUint32Le(4, 16))
	require.Equal(t, []byte{0, 0, 0, 0, 16, 0, 0, 0}, mem.Buffer)
	require.False(t, mem.WriteUint32Le(5, 16))
	require.False(t, mem.WriteUint32Le(9, 16))
}
