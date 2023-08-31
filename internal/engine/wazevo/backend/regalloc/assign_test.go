package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAllocator_assignRegistersPerInstr(t *testing.T) {
	t.Run("call", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{CallerSavedRegisters: map[RealReg]struct{}{1: {}, 3: {}}})
		pc := programCounter(5)
		liveNodes := []liveNodeInBlock{
			{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: RealRegInvalid, v: 0xb, ranges: []liveRange{{begin: 5, end: 20}}}},           // Spill. not save target.
			{n: &node{r: 2, v: FromRealReg(1, RegTypeInt), ranges: []liveRange{{begin: 5, end: 20}}}}, // Real reg-backed VReg. not save target
			{n: &node{r: 3, v: 0xc, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: 4, v: 0xd, ranges: []liveRange{{begin: 5, end: 20}}}}, // real reg, but not caller saved. not save target
		}
		call := newMockInstr().asCall()
		blk := newMockBlock(0, call).entry()
		f := newMockFunction(blk)
		a.assignRegistersPerInstr(f, pc, call, nil, liveNodes)

		require.Equal(t, 2, len(f.storeRegisterBefore))
		require.Equal(t, 2, len(f.reloadRegisterAfter))
	})
	t.Run("call_indirect/func_ptr not spilled", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{CallerSavedRegisters: map[RealReg]struct{}{1: {}, 3: {}, 0xff: {}}})
		pc := programCounter(5)
		functionPtrVRegID := 0x0
		functionPtrVReg := VReg(functionPtrVRegID).SetRegType(RegTypeInt)
		functionPtrLiveNode := liveNodeInBlock{
			n: &node{r: 0xff, v: functionPtrVReg, ranges: []liveRange{{begin: 4, end: pc /* killed at this indirect call. */}}},
		}
		liveNodes := []liveNodeInBlock{
			functionPtrLiveNode, // Function pointer, used at this PC. not save target.
			{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: 2, v: FromRealReg(1, RegTypeInt), ranges: []liveRange{{begin: 5, end: 20}}}}, // Real reg-backed VReg. not target
			{n: &node{r: 3, v: 0xc, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: 4, v: 0xd, ranges: []liveRange{{begin: 5, end: 20}}}}, // real reg, but not caller saved. not save target
		}
		callInd := newMockInstr().asIndirectCall().use(functionPtrVReg)
		blk := newMockBlock(0, callInd).entry()
		f := newMockFunction(blk)
		a.assignRegistersPerInstr(f, pc, callInd, []*node{0: functionPtrLiveNode.n}, liveNodes)

		require.Equal(t, 2, len(f.storeRegisterBefore))
		require.Equal(t, 2, len(f.reloadRegisterAfter))
	})
	t.Run("call_indirect/func_ptr spilled", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{
			CallerSavedRegisters: map[RealReg]struct{}{1: {}, 3: {}, 0xbb: {}},
			AllocatableRegisters: [3][]RealReg{RegTypeInt: {0xff, 0xbb}},
		})
		pc := programCounter(5)
		functionPtrVRegID := 0x0
		functionPtrVReg := VReg(functionPtrVRegID).SetRegType(RegTypeInt)
		liveNodes := []liveNodeInBlock{
			{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: 2, v: FromRealReg(1, RegTypeInt), ranges: []liveRange{{begin: 5, end: 20}}}}, // Real reg-backed VReg. not target
			{n: &node{r: 3, v: 0xc, ranges: []liveRange{{begin: 5, end: 20}}}},
			{n: &node{r: 4, v: 0xd, ranges: []liveRange{{begin: 5, end: 20}}}}, // real reg, but not caller saved. not save target
		}
		callInd := newMockInstr().asIndirectCall().use(functionPtrVReg)
		blk := newMockBlock(0, callInd).entry()
		f := newMockFunction(blk)
		a.assignRegistersPerInstr(f, pc, callInd, []*node{
			0: {r: RealRegInvalid},
		}, liveNodes)

		require.Equal(t, 2, len(f.storeRegisterBefore))
		require.Equal(t, 2, len(f.reloadRegisterAfter))
		require.Equal(t, 1, len(f.reloadRegisterBefore))
		require.Equal(t, callInd, f.reloadRegisterBefore[0].instr)
		require.Equal(t, functionPtrVReg.SetRealReg(0xbb), f.reloadRegisterBefore[0].v)
	})
	t.Run("no spills", func(t *testing.T) {
		r := FromRealReg(1, RegTypeInt)
		a := NewAllocator(&RegisterInfo{})
		instr := newMockInstr().def(4).use(r, 2, 3)
		blk := newMockBlock(0, instr).entry()
		f := newMockFunction(blk)

		a.assignRegistersPerInstr(f, 0, instr, []*node{
			2: {r: 22},
			3: {r: 33},
			4: {r: 44},
		}, nil)

		require.Equal(t, []VReg{r, VReg(2).SetRealReg(22), VReg(3).SetRealReg(33)}, instr.uses)
		require.Equal(t, []VReg{VReg(4).SetRealReg(44)}, instr.defs)
	})
	t.Run("spills", func(t *testing.T) {
		t.Skip("TODO")
	})
}

func TestAllocator_activeNonRealVRegsAt(t *testing.T) {
	r := FromRealReg(1, RegTypeInt)
	for _, tc := range []struct {
		name  string
		lives []liveNodeInBlock
		pc    programCounter
		want  []VReg
	}{
		{
			name:  "no live nodes",
			pc:    0,
			lives: []liveNodeInBlock{},
			want:  []VReg{},
		},
		{
			name:  "no live nodes at pc",
			pc:    10,
			lives: []liveNodeInBlock{{n: &node{ranges: []liveRange{{begin: 100, end: 2000}}}}},
			want:  []VReg{},
		},
		{
			name: "one live",
			pc:   10,
			lives: []liveNodeInBlock{
				{n: &node{r: 2, v: 0xf, ranges: []liveRange{{begin: 5, end: 20}}}},
				{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 100, end: 2000}}}},
			},
			want: []VReg{0xf},
		},
		{
			name: "three lives but one spill",
			pc:   10,
			lives: []liveNodeInBlock{
				{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 5, end: 20}}}},
				{n: &node{r: RealRegInvalid, v: 0xb, ranges: []liveRange{{begin: 5, end: 20}}}}, // Spill.
				{n: &node{r: 3, v: 0xc, ranges: []liveRange{{begin: 5, end: 20}}}},
			},
			want: []VReg{0xa, 0xc},
		},
		{
			name: "three lives but one real reg-backed VReg",
			pc:   10,
			lives: []liveNodeInBlock{
				{n: &node{r: 1, v: 0xa, ranges: []liveRange{{begin: 5, end: 20}}}},
				{n: &node{r: 2, v: r, ranges: []liveRange{{begin: 5, end: 20}}}}, // Real reg-backed VReg.
				{n: &node{r: 3, v: 0xc, ranges: []liveRange{{begin: 5, end: 20}}}},
			},
			want: []VReg{0xa, 0xc},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a := NewAllocator(&RegisterInfo{})
			ans := a.collectActiveNonRealVRegsAt(nil, tc.pc, tc.lives)

			actual := make([]VReg, len(ans))
			for i, n := range ans {
				actual[i] = n.v
			}
			require.Equal(t, tc.want, actual)
		})
	}
}
