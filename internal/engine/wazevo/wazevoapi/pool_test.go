package wazevoapi

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewNilVarLength(t *testing.T) {
	v := NewNilVarLength[uint64]()
	require.NotNil(t, v)
	pool := NewVarLengthPool[uint64]()
	v = v.Append(&pool, 1)
	require.Equal(t, []uint64{1}, v.View())
}

func TestAllocate(t *testing.T) {
	pool := NewVarLengthPool[uint64]()
	// Array:
	v := pool.Allocate(5)
	require.NotNil(t, v.arr)
	require.Equal(t, 0, v.arr.next)
	require.Equal(t, arraySize, cap(v.arr.arr))

	// Slice backed:
	v = pool.Allocate(25)
	require.NotNil(t, v.slc)
	require.Equal(t, 0, len(*v.slc))
	v.Append(&pool, 1)
	require.NotNil(t, v.slc)
	require.Equal(t, 1, len(*v.slc))
	v.Append(&pool, 2)
	require.NotNil(t, v.slc)
	require.Equal(t, 2, len(*v.slc))
	capacity := cap(*v.slc)

	// Reset the pool and ensure the backing slice is reused.
	pool.Reset()

	v = pool.Allocate(5)
	require.NotNil(t, v.arr)
	require.Equal(t, 0, v.arr.next)
	require.Equal(t, arraySize, cap(v.arr.arr))
	v = pool.Allocate(25)
	require.NotNil(t, v.slc)
	require.Equal(t, 0, len(*v.slc))
	require.Equal(t, capacity, cap(*v.slc))
}

func TestAppendAndView(t *testing.T) {
	pool := NewVarLengthPool[uint64]()
	t.Run("zero start", func(t *testing.T) {
		v := pool.Allocate(0)
		v.Append(&pool, 1)
		require.Equal(t, []uint64{1}, v.View())
	})
	t.Run("non zero start", func(t *testing.T) {
		v := pool.Allocate(10)
		for i := uint64(0); i < 10; i++ {
			v.Append(&pool, i)
		}
		require.Equal(t, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, v.View())
		for i := uint64(0); i < 20; i++ {
			v.Append(&pool, i)
		}
		require.Equal(t, []uint64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
			0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x11, 0x12, 0x13,
		}, v.View())
	})
}

func TestCut(t *testing.T) {
	pool := NewVarLengthPool[uint64]()
	v := pool.Allocate(10)
	for i := uint64(0); i < 10; i++ {
		v.Append(&pool, i)
	}
	v.Cut(5)
	require.Equal(t, []uint64{0, 1, 2, 3, 4}, v.View())
}
