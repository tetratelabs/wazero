package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestRegTypeOf(t *testing.T) {
	require.Equal(t, RegTypeInt, RegTypeOf(ssa.TypeI32))
	require.Equal(t, RegTypeInt, RegTypeOf(ssa.TypeI64))
	require.Equal(t, RegTypeFloat, RegTypeOf(ssa.TypeF32))
	require.Equal(t, RegTypeFloat, RegTypeOf(ssa.TypeF64))
}

func TestVReg_String(t *testing.T) {
	require.Equal(t, "v0?", VReg(0).String())
	require.Equal(t, "v100?", VReg(100).String())
	require.Equal(t, "r5", FromRealReg(5, RegTypeInt).String())
}

func Test_FromRealReg(t *testing.T) {
	r := FromRealReg(5, RegTypeInt)
	require.Equal(t, RealReg(5), r.RealReg())
	require.Equal(t, VRegID(5), r.ID())
}

func TestVRegTable(t *testing.T) {
	min := VRegIDMinSet{}
	min.Observe(VReg(vRegIDReservedForRealNum + 2))
	min.Observe(VReg(vRegIDReservedForRealNum + 1))
	min.Observe(VReg(vRegIDReservedForRealNum + 0))

	table := VRegTable{}
	table.Reset(min)
	table.Insert(VReg(vRegIDReservedForRealNum+0), 1)
	table.Insert(VReg(vRegIDReservedForRealNum+1), 10)
	table.Insert(VReg(vRegIDReservedForRealNum+2), 100)

	vregs := map[VReg]programCounter{}
	table.Range(func(v VReg, p programCounter) {
		vregs[v] = p
	})
	require.Equal(t, 3, len(vregs))

	for v, p := range vregs {
		require.True(t, table.Contains(v))
		require.Equal(t, p, table.Lookup(v))
	}

	table.Range(func(v VReg, p programCounter) {
		require.Equal(t, vregs[v], p)
		delete(vregs, v)
	})
	require.Equal(t, 0, len(vregs))
}
