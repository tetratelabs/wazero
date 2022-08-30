package wasm

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryPageConsts(t *testing.T) {
	require.Equal(t, MemoryPageSize, uint32(1)<<MemoryPageSizeInBits)
	require.Equal(t, MemoryPageSize, uint32(1<<16))
	require.Equal(t, MemoryLimitPages, uint32(1<<16))
}

func TestMemoryPagesToBytesNum(t *testing.T) {
	for _, numPage := range []uint32{0, 1, 5, 10} {
		require.Equal(t, uint64(numPage*MemoryPageSize), MemoryPagesToBytesNum(numPage))
	}
}

func TestMemoryBytesNumToPages(t *testing.T) {
	for _, numbytes := range []uint32{0, MemoryPageSize * 1, MemoryPageSize * 10} {
		require.Equal(t, numbytes/MemoryPageSize, memoryBytesNumToPages(uint64(numbytes)))
	}
}

func TestPagesToUnitOfBytes(t *testing.T) {
	tests := []struct {
		name     string
		pages    uint32
		expected string
	}{
		{
			name:     "zero",
			pages:    0,
			expected: "0 Ki",
		},
		{
			name:     "one",
			pages:    1,
			expected: "64 Ki",
		},
		{
			name:     "megs",
			pages:    100,
			expected: "6 Mi",
		},
		{
			name:     "max memory",
			pages:    MemoryLimitPages,
			expected: "4 Gi",
		},
		{
			name:     "max uint32",
			pages:    math.MaxUint32,
			expected: "3 Ti",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, PagesToUnitOfBytes(tc.pages))
		})
	}
}
