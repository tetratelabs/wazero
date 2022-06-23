package asm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewStaticConstPool(t *testing.T) {
	p := NewStaticConstPool()
	require.NotNil(t, p.addedConsts)
}

func TestStaticConst_AddOffsetFinalizedCallback(t *testing.T) {
	p := NewStaticConstPool()
	const firstUseOffset uint64 = 100

	// Add first const.
	c := NewStaticConst([]byte{1})
	p.AddConst(c, firstUseOffset)
	require.Equal(t, firstUseOffset, *p.FirstUseOffsetInBinary)
	require.Equal(t, 1, len(p.Consts))
	require.Equal(t, 1, len(p.addedConsts))

	// Adding the same *StaticConst doesn't affect the state.
	p.AddConst(c, firstUseOffset+10000)
	require.Equal(t, firstUseOffset, *p.FirstUseOffsetInBinary)
	require.Equal(t, 1, len(p.Consts))
	require.Equal(t, 1, len(p.addedConsts))

	// Add another const.
	c2 := NewStaticConst([]byte{1, 2})
	p.AddConst(c2, firstUseOffset+100)
	require.Equal(t, firstUseOffset, *p.FirstUseOffsetInBinary) // first use doesn't change!
	require.Equal(t, 2, len(p.Consts))
	require.Equal(t, 2, len(p.addedConsts))
}

func TestStaticConst_SetOffsetInBinary(t *testing.T) {
	sc := NewStaticConst([]byte{1})
	const offset uint64 = 100
	sc.AddOffsetFinalizedCallback(func(offsetOfConstInBinary uint64) {
		require.Equal(t, offset, offsetOfConstInBinary)
	})
	sc.SetOffsetInBinary(offset)
}
