//go:build tmp

package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAllocator_assignRegistersPerInstr(t *testing.T) {
	t.Run("call", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{CallerSavedRegisters: [RealRegsNumMax]bool{1: true, 3: true}})
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

		require.Equal(t, 2, len(f.befores))
		require.Equal(t, 2, len(f.afters))
	})
	t.Run("call_indirect/func_ptr not spilled", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{CallerSavedRegisters: [RealRegsNumMax]bool{1: true, 3: true, 0xf: true}})
		pc := programCounter(5)
		functionPtrVRegID := 0x0
		functionPtrVReg := VReg(functionPtrVRegID).SetRegType(RegTypeInt)
		functionPtrLiveNode := liveNodeInBlock{
			n: &node{r: 0xf, v: functionPtrVReg, ranges: []liveRange{{begin: 4, end: pc /* killed at this indirect call. */}}},
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

		require.Equal(t, 2, len(f.befores))
		require.Equal(t, 2, len(f.afters))
		require.True(t, callInd.uses[0].IsRealReg())
		require.Equal(t, functionPtrVReg.SetRealReg(0xf), callInd.uses[0])
	})
	t.Run("call_indirect/func_ptr spilled", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{
			CallerSavedRegisters: [RealRegsNumMax]bool{1: true, 3: true, 0xb: true},
			AllocatableRegisters: [3][]RealReg{RegTypeInt: {0xf, 0xb}},
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

		require.Equal(t, 3, len(f.befores))
		require.Equal(t, 2, len(f.afters))
		require.Equal(t, callInd, f.befores[2].instr)
		require.Equal(t, functionPtrVReg.SetRealReg(0xb), f.befores[2].v)
		require.True(t, callInd.uses[0].IsRealReg())
		require.Equal(t, functionPtrVReg.SetRealReg(0xb), callInd.uses[0])
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
}

func TestAllocator_handleSpills(t *testing.T) {
	requireInsertedInst := func(t *testing.T, f *mockFunction, before bool, index int, instr Instr, reload bool, v VReg) {
		lists := f.afters
		if before {
			lists = f.befores
		}
		actual := lists[index]
		require.Equal(t, instr, actual.instr)
		require.Equal(t, reload, actual.reload)
		require.Equal(t, v, actual.v)
	}

	t.Run("no spills", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		a.handleSpills(nil, 0, nil, nil, nil, VRegInvalid)
	})
	t.Run("only def / evicted / Real reg backed VReg", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			// Real reg backed VReg.
			{n: &node{r: RealRegInvalid, v: VReg(1).SetRealReg(0xa), ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xc), v: 0xc, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{RegTypeInt: {0xa, 0xb, 0xc}}, // Only live nodes are allocatable.
		})

		f := newMockFunction(newMockBlock(0).entry())

		vr := VReg(100).SetRegType(RegTypeInt)
		instr := newMockInstr().def(vr)
		a.handleSpills(f, pc, instr, liveNodes, nil, vr)
		require.Equal(t, 1, len(instr.defs))
		require.Equal(t, RealReg(0xa), instr.defs[0].RealReg())

		require.Equal(t, 1, len(f.befores))
		requireInsertedInst(t, f, true, 0, instr, false, liveNodes[0].n.v)

		require.Equal(t, 2, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, true, liveNodes[0].n.v)
		requireInsertedInst(t, f, false, 1, instr, false, vr.SetRealReg(0xa))
	})

	t.Run("only def / evicted", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xc), v: 0xc, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{RegTypeInt: {0xb, 0xc}}, // Only live nodes are allocatable.
		})

		f := newMockFunction(newMockBlock(0).entry())

		vr := VReg(100).SetRegType(RegTypeInt)
		instr := newMockInstr().def(vr)
		a.handleSpills(f, pc, instr, liveNodes, nil, vr)
		require.Equal(t, 1, len(instr.defs))
		require.Equal(t, RealReg(0xb), instr.defs[0].RealReg())

		require.Equal(t, 1, len(f.befores))
		requireInsertedInst(t, f, true, 0, instr, false, liveNodes[0].n.v.SetRealReg(0xb))

		require.Equal(t, 2, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, true, liveNodes[0].n.v.SetRealReg(0xb))
		requireInsertedInst(t, f, false, 1, instr, false, vr.SetRealReg(0xb))
	})

	t.Run("only def / not evicted", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xc), v: 0xc, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{RegTypeInt: {0xb, 0xc, 0xf /* free */}},
		})

		f := newMockFunction(newMockBlock(0).entry())

		vr := VReg(100).SetRegType(RegTypeInt)
		instr := newMockInstr().def(vr)
		a.handleSpills(f, pc, instr, liveNodes, nil, vr)
		require.Equal(t, 1, len(instr.defs))
		require.Equal(t, RealReg(0xf), instr.defs[0].RealReg())

		require.Equal(t, 0, len(f.befores))
		require.Equal(t, 1, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, false, vr.SetRealReg(0xf))
	})

	t.Run("uses and def / not evicted / def same type", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xc), v: 0xc, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{
				RegTypeInt:   {0xb, 0xc, 0xa /* free */},
				RegTypeFloat: {0xf /* free */},
			},
		})

		f := newMockFunction(newMockBlock(0).entry())
		u1, u2, u3 := VReg(100).SetRegType(RegTypeInt),
			VReg(101).SetRegType(RegTypeInt).SetRealReg(0x88), // This one isn't spilled.
			VReg(102).SetRegType(RegTypeFloat)
		d1 := VReg(104).SetRegType(RegTypeFloat)
		instr := newMockInstr().use(u1, u2, u3).def(d1)
		a.handleSpills(f, pc, instr, liveNodes, []VReg{u1, u3}, d1)
		require.Equal(t, []VReg{u1.SetRealReg(0xa), u2, u3.SetRealReg(0xf)}, instr.uses)
		require.Equal(t, []VReg{d1.SetRealReg(0xf)}, instr.defs)

		require.Equal(t, 2, len(f.befores))
		requireInsertedInst(t, f, true, 0, instr, true, u1.SetRealReg(0xa))
		requireInsertedInst(t, f, true, 1, instr, true, u3.SetRealReg(0xf))

		require.Equal(t, 1, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, false, d1.SetRealReg(0xf))
	})

	t.Run("uses and def / not evicted / def different type", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xf), v: 0xb, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{
				RegTypeInt:   {0xb, 0xa /* free */},
				RegTypeFloat: {0xf},
			},
		})

		f := newMockFunction(newMockBlock(0).entry())
		u1 := VReg(100).SetRegType(RegTypeInt)
		d1 := VReg(104).SetRegType(RegTypeFloat)
		instr := newMockInstr().use(u1).def(d1)
		a.handleSpills(f, pc, instr, liveNodes, []VReg{u1}, d1)
		require.Equal(t, []VReg{u1.SetRealReg(0xa)}, instr.uses)
		require.Equal(t, []VReg{d1.SetRealReg(0xf)}, instr.defs)

		require.Equal(t, 2, len(f.befores))
		requireInsertedInst(t, f, true, 0, instr, true, u1.SetRealReg(0xa))
		requireInsertedInst(t, f, true, 1, instr, false, liveNodes[1].n.v.SetRealReg(0xf))
		require.Equal(t, 2, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, true, liveNodes[1].n.v.SetRealReg(0xf))
		requireInsertedInst(t, f, false, 1, instr, false, d1.SetRealReg(0xf))
	})

	t.Run("uses and def / evicted / def different type", func(t *testing.T) {
		const pc = 5
		liveNodes := []liveNodeInBlock{
			{n: &node{r: RealReg(0xb), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xc), v: 0xa, ranges: []liveRange{{begin: pc, end: 20}}}},
			{n: &node{r: RealReg(0xf), v: 0xb, ranges: []liveRange{{begin: pc, end: 20}}}},
		}

		a := NewAllocator(&RegisterInfo{
			AllocatableRegisters: [3][]RealReg{
				RegTypeInt:   {0xb, 0xc},
				RegTypeFloat: {0xf},
			},
		})

		f := newMockFunction(newMockBlock(0).entry())
		u1 := VReg(100).SetRegType(RegTypeInt)
		d1 := VReg(104).SetRegType(RegTypeFloat)
		instr := newMockInstr().use(u1).def(d1)
		a.handleSpills(f, pc, instr, liveNodes, []VReg{u1}, d1)
		require.Equal(t, []VReg{u1.SetRealReg(0xb)}, instr.uses)
		require.Equal(t, []VReg{d1.SetRealReg(0xf)}, instr.defs)

		require.Equal(t, 3, len(f.befores))
		requireInsertedInst(t, f, true, 0, instr, false, liveNodes[0].n.v.SetRealReg(0xb))
		requireInsertedInst(t, f, true, 1, instr, true, u1.SetRealReg(0xb))
		requireInsertedInst(t, f, true, 2, instr, false, liveNodes[2].n.v.SetRealReg(0xf))
		require.Equal(t, 3, len(f.afters))
		requireInsertedInst(t, f, false, 0, instr, true, liveNodes[0].n.v.SetRealReg(0xb))
		requireInsertedInst(t, f, false, 1, instr, true, liveNodes[2].n.v.SetRealReg(0xf))
		requireInsertedInst(t, f, false, 2, instr, false, d1.SetRealReg(0xf))
	})
}
