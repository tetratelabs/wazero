package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAllocator_livenessAnalysis(t *testing.T) {
	const realRegID, realRegID2 = 50, 100
	realReg, realReg2 := FromRealReg(realRegID, RegTypeInt), FromRealReg(realRegID2, RegTypeInt)
	const phiVReg = 12345
	for _, tc := range []struct {
		name  string
		setup func() Function
		exp   map[int]*blockInfo
	}{
		{
			name: "single block",
			setup: func() Function {
				return newMockFunction(
					newMockBlock(0,
						newMockInstr().def(1),
						newMockInstr().use(1).def(2),
					).entry(),
				)
			},
			exp: map[int]*blockInfo{
				0: {
					defs:     map[VReg]programCounter{2: pcDefOffset + pcStride, 1: pcDefOffset},
					lastUses: map[VReg]programCounter{1: pcStride + pcUseOffset},
					kills:    map[VReg]programCounter{1: pcStride + pcUseOffset},
				},
			},
		},
		{
			name: "straight",
			// b0 -> b1 -> b2
			setup: func() Function {
				b0 := newMockBlock(0,
					newMockInstr().def(1000, 1, 2),
					newMockInstr().use(1000),
					newMockInstr().use(1, 2).def(3),
				).entry()
				b1 := newMockBlock(1,
					newMockInstr().def(realReg),
					newMockInstr().use(3).def(4, 5),
					newMockInstr().use(realReg),
				)
				b2 := newMockBlock(2,
					newMockInstr().use(3, 4, 5),
				)
				b2.addPred(b1)
				b1.addPred(b0)
				return newMockFunction(b0, b1, b2)
			},
			exp: map[int]*blockInfo{
				0: {
					defs: map[VReg]programCounter{
						1000: pcDefOffset,
						1:    pcDefOffset,
						2:    pcDefOffset,
						3:    pcStride*2 + pcDefOffset,
					},
					lastUses: map[VReg]programCounter{
						1000: pcStride + pcUseOffset,
						1:    pcStride*2 + pcUseOffset,
						2:    pcStride*2 + pcUseOffset,
					},
					liveOuts: map[VReg]struct{}{3: {}},
					kills: map[VReg]programCounter{
						1000: pcStride + pcUseOffset,
						1:    pcStride*2 + pcUseOffset,
						2:    pcStride*2 + pcUseOffset,
					},
				},
				1: {
					liveIns:  map[VReg]struct{}{3: {}},
					liveOuts: map[VReg]struct{}{3: {}, 4: {}, 5: {}},
					lastUses: map[VReg]programCounter{
						3: pcStride + pcUseOffset,
					},
					defs: map[VReg]programCounter{
						4: pcStride + pcDefOffset,
						5: pcStride + pcDefOffset,
					},
					realRegUses: [vRegIDReservedForRealNum][]programCounter{
						realRegID: {pcStride*2 + pcUseOffset},
					},
					realRegDefs: [vRegIDReservedForRealNum][]programCounter{
						realRegID: {pcDefOffset},
					},
				},
				2: {
					liveIns:  map[VReg]struct{}{3: {}, 4: {}, 5: {}},
					lastUses: map[VReg]programCounter{3: pcUseOffset, 4: pcUseOffset, 5: pcUseOffset},
					kills:    map[VReg]programCounter{3: pcUseOffset, 4: pcUseOffset, 5: pcUseOffset},
				},
			},
		},
		{
			name: "diamond",
			//  0   v1000<-, v1<-, v2<-
			// / \
			// 1   2
			// \ /
			//  3
			setup: func() Function {
				b0 := newMockBlock(0,
					newMockInstr().def(1000),
					newMockInstr().def(1),
					newMockInstr().def(2),
				).entry()
				b1 := newMockBlock(1,
					newMockInstr().def(realReg).use(1),
					newMockInstr().use(realReg),
					newMockInstr().def(realReg2),
					newMockInstr().use(realReg2),
					newMockInstr().def(realReg),
					newMockInstr().use(realReg),
				)
				b2 := newMockBlock(2,
					newMockInstr().use(2, realReg2),
				)
				b3 := newMockBlock(3,
					newMockInstr().use(1000),
				)
				b3.addPred(b1)
				b3.addPred(b2)
				b1.addPred(b0)
				b2.addPred(b0)
				return newMockFunction(b0, b1, b2, b3)
			},
			exp: map[int]*blockInfo{
				0: {
					defs: map[VReg]programCounter{
						1000: pcDefOffset,
						1:    pcStride + pcDefOffset,
						2:    pcStride*2 + pcDefOffset,
					},
					liveOuts: map[VReg]struct{}{1000: {}, 1: {}, 2: {}},
				},
				1: {
					liveIns:  map[VReg]struct{}{1000: {}, 1: {}},
					liveOuts: map[VReg]struct{}{1000: {}},
					lastUses: map[VReg]programCounter{1: pcUseOffset},
					kills:    map[VReg]programCounter{1: pcUseOffset},
					realRegDefs: [vRegIDReservedForRealNum][]programCounter{
						realRegID:  {pcDefOffset, pcStride*4 + pcDefOffset},
						realRegID2: {pcStride*2 + pcDefOffset},
					},
					realRegUses: [vRegIDReservedForRealNum][]programCounter{
						realRegID:  {pcStride + pcUseOffset, pcStride*5 + pcUseOffset},
						realRegID2: {pcStride*3 + pcUseOffset},
					},
				},
				2: {
					liveIns:     map[VReg]struct{}{1000: {}, 2: {}},
					liveOuts:    map[VReg]struct{}{1000: {}},
					lastUses:    map[VReg]programCounter{2: pcUseOffset},
					kills:       map[VReg]programCounter{2: pcUseOffset},
					realRegUses: [vRegIDReservedForRealNum][]programCounter{realRegID2: {pcUseOffset}},
					realRegDefs: [vRegIDReservedForRealNum][]programCounter{realRegID2: {0}},
				},
				3: {
					liveIns:  map[VReg]struct{}{1000: {}},
					lastUses: map[VReg]programCounter{1000: pcUseOffset},
					kills:    map[VReg]programCounter{1000: pcUseOffset},
				},
			},
		},
		{
			name: "phis",
			//   0
			// /  \
			// 1   \
			// |   |
			// 2   3
			//  \ /
			//   4  use v5 (phi node) defined at both 1 and 3.
			setup: func() Function {
				b0 := newMockBlock(0,
					newMockInstr().def(1000, 2000, 3000),
				).entry()
				b1 := newMockBlock(1,
					newMockInstr().def(phiVReg).use(2000),
				)
				b2 := newMockBlock(2)
				b3 := newMockBlock(3,
					newMockInstr().def(phiVReg).use(1000),
				)
				b4 := newMockBlock(
					4, newMockInstr().use(phiVReg, 3000),
				)
				b4.addPred(b2)
				b4.addPred(b3)
				b3.addPred(b0)
				b2.addPred(b1)
				b1.addPred(b0)
				return newMockFunction(b0, b1, b2, b3, b4)
			},
			exp: map[int]*blockInfo{
				0: {
					defs:     map[VReg]programCounter{1000: pcDefOffset, 2000: pcDefOffset, 3000: pcDefOffset},
					liveOuts: map[VReg]struct{}{1000: {}, 2000: {}, 3000: {}},
				},
				1: {
					liveIns:  map[VReg]struct{}{2000: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: map[VReg]programCounter{2000: pcUseOffset},
					kills:    map[VReg]programCounter{2000: pcUseOffset},
				},
				2: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
				},
				3: {
					liveIns:  map[VReg]struct{}{1000: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: map[VReg]programCounter{1000: pcUseOffset},
					kills:    map[VReg]programCounter{1000: pcUseOffset},
				},
				4: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 3000: {}},
					lastUses: map[VReg]programCounter{phiVReg: pcUseOffset, 3000: pcUseOffset},
					kills:    map[VReg]programCounter{phiVReg: pcUseOffset, 3000: pcUseOffset},
				},
			},
		},
		{
			name: "loop",
			// 0 -> 1 -> 2
			//      ^    |
			//      |    v
			//      4 <- 3 -> 5
			setup: func() Function {
				b0 := newMockBlock(0,
					newMockInstr().def(1),
					newMockInstr().def(phiVReg).use(1),
				).entry()
				b1 := newMockBlock(1,
					newMockInstr().def(9999),
				)
				b2 := newMockBlock(2,
					newMockInstr().def(100).use(phiVReg, 9999),
				)
				b3 := newMockBlock(3,
					newMockInstr().def(54321),
					newMockInstr().use(100),
				)
				b4 := newMockBlock(4,
					newMockInstr().def(phiVReg).use(54321),
				)
				b5 := newMockBlock(
					4, newMockInstr().use(54321),
				)
				b1.addPred(b0)
				b1.addPred(b4)
				b2.addPred(b1)
				b3.addPred(b2)
				b4.addPred(b3)
				b5.addPred(b3)
				return newMockFunction(b0, b1, b2, b3, b4, b5)
			},
			exp: map[int]*blockInfo{
				0: {
					liveIns: map[VReg]struct{}{},
					liveOuts: map[VReg]struct{}{
						phiVReg: {},
					},
					defs: map[VReg]programCounter{
						1:       pcDefOffset,
						phiVReg: pcStride + pcDefOffset,
					},
					lastUses: map[VReg]programCounter{
						1: pcStride + pcUseOffset,
					},
					kills: map[VReg]programCounter{
						1: pcStride + pcUseOffset,
					},
				},
				1: {
					liveIns:  map[VReg]struct{}{phiVReg: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 9999: {}},
					defs:     map[VReg]programCounter{9999: pcDefOffset},
					lastUses: map[VReg]programCounter{},
					kills:    map[VReg]programCounter{},
				},
				2: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 9999: {}},
					liveOuts: map[VReg]struct{}{100: {}},
					defs:     map[VReg]programCounter{100: pcDefOffset},
					lastUses: map[VReg]programCounter{phiVReg: pcUseOffset, 9999: pcUseOffset},
					kills:    map[VReg]programCounter{phiVReg: pcUseOffset, 9999: pcUseOffset},
				},
				3: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{54321: {}},
					defs:     map[VReg]programCounter{54321: pcDefOffset},
					lastUses: map[VReg]programCounter{100: pcStride + pcUseOffset},
					kills:    map[VReg]programCounter{100: pcStride + pcUseOffset},
				},
				4: {
					liveIns:  map[VReg]struct{}{54321: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: map[VReg]programCounter{54321: pcUseOffset},
					kills:    map[VReg]programCounter{54321: pcUseOffset},
				},
			},
		},
		{
			//           -----+
			//           v    |
			// 0 -> 1 -> 2 -> 3 -> 4
			//      ^    |
			//      +----+
			name: "Fig. 9.2 in paper",
			setup: func() Function {
				b0 := newMockBlock(0, newMockInstr().def(99999)).entry()
				b1 := newMockBlock(1, newMockInstr().use(99999))
				b2 := newMockBlock(2)
				b3 := newMockBlock(3)
				b4 := newMockBlock(4)
				b1.addPred(b0)
				b1.addPred(b2)
				b2.addPred(b1)
				b2.addPred(b3)
				b3.addPred(b2)
				b4.addPred(b3)
				return newMockFunction(b0, b1, b2, b3, b4)
			},
			exp: map[int]*blockInfo{
				0: {
					defs:     map[VReg]programCounter{99999: pcDefOffset},
					liveOuts: map[VReg]struct{}{99999: {}},
				},
				1: {
					liveIns:  map[VReg]struct{}{99999: {}},
					liveOuts: map[VReg]struct{}{99999: {}},
					lastUses: map[VReg]programCounter{99999: pcUseOffset},
				},
				2: {
					liveIns:  map[VReg]struct{}{99999: {}},
					liveOuts: map[VReg]struct{}{99999: {}},
				},
				3: {
					liveIns:  map[VReg]struct{}{99999: {}},
					liveOuts: map[VReg]struct{}{99999: {}},
				},
				4: {},
			},
		},
		//      2
		//      ^              +----+
		//      |              v    |
		// 0 -> 1 -> 3 -> 4 -> 5 -> 6 -> 9
		//      ^    |         ^         |
		//      |    v         |         |
		//      |    7 -> 8 ---+         |
		//      |    ^    |              |
		//      |    +----+              |
		//      +------------------------+
		{
			name: "Fig. 9.1 in paper",
			setup: func() Function {
				b0 := newMockBlock(0).entry()
				b1 := newMockBlock(1)
				b2 := newMockBlock(2)
				b3 := newMockBlock(3,
					newMockInstr().def(100),
				)
				b4 := newMockBlock(4)
				b5 := newMockBlock(5,
					newMockInstr().use(100),
				)
				b6 := newMockBlock(6)
				b7 := newMockBlock(7)
				b8 := newMockBlock(8)
				b9 := newMockBlock(9)

				b1.addPred(b0)
				b1.addPred(b9)

				b2.addPred(b1)

				b3.addPred(b1)

				b4.addPred(b3)

				b5.addPred(b4)
				b5.addPred(b6)
				b5.addPred(b8)

				b6.addPred(b5)

				b7.addPred(b3)
				b7.addPred(b8)

				b8.addPred(b7)

				b9.addPred(b6)
				return newMockFunction(b0, b1, b2, b3, b4, b7, b8, b5, b6, b9)
			},
			exp: map[int]*blockInfo{
				0: {},
				1: {},
				2: {},
				3: {
					liveOuts: map[VReg]struct{}{100: {}},
					defs:     map[VReg]programCounter{100: pcDefOffset},
				},
				4: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{100: {}},
				},
				5: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{100: {}},
					lastUses: map[VReg]programCounter{100: pcUseOffset},
				},
				6: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{100: {}},
				},
				7: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{100: {}},
				},
				8: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{100: {}},
				},
				9: {},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := tc.setup()
			a := NewAllocator(&RegisterInfo{})
			a.livenessAnalysis(f)
			for blockID := range a.blockInfos {
				actual := &a.blockInfos[blockID]
				exp := tc.exp[blockID]
				initMapInInfo(exp)
				saved := actual.intervalMng
				actual.intervalMng = nil // Don't compare intervalManager.
				require.Equal(t, exp, actual, "\n[exp for block[%d]]\n%s\n[actual for block[%d]]\n%s", blockID, exp, blockID, actual)
				actual.intervalMng = saved
			}

			// Sanity check: buildLiveRanges should not panic.
			a.buildLiveRanges(f)
		})
	}
}

func TestAllocator_livenessAnalysis_copy(t *testing.T) {
	f := newMockFunction(
		newMockBlock(0,
			newMockInstr().def(1),
			newMockInstr().use(1).def(2).asCopy(),
		).entry(),
	)
	a := NewAllocator(&RegisterInfo{})
	a.livenessAnalysis(f)

	n1, n2 := a.getOrAllocateNode(1), a.getOrAllocateNode(2)
	require.Equal(t, n2, n1.copyToVReg)
	require.Equal(t, n2.copyFromVReg, n1)
	require.Nil(t, n1.copyFromVReg)
	require.Nil(t, n2.copyToVReg)
}

func TestAllocator_recordCopyRelation(t *testing.T) {
	t.Run("real/real", func(t *testing.T) {
		// Just ensure that it doesn't panic.
		a := NewAllocator(&RegisterInfo{})
		a.recordCopyRelation(FromRealReg(1, RegTypeInt), FromRealReg(2, RegTypeInt))
	})
	t.Run("read/virtual", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		v100, r := VReg(100), FromRealReg(1, RegTypeInt)
		a.recordCopyRelation(v100, r)

		n := a.vRegIDToNode[100]
		require.Nil(t, n.copyFromVReg)
		require.Nil(t, n.copyToVReg)
		require.Equal(t, RealRegInvalid, n.copyToReal)
		require.Equal(t, r.RealReg(), n.copyFromReal)
	})
	t.Run("virtual/read", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		v100, r := VReg(100), FromRealReg(1, RegTypeInt)
		a.recordCopyRelation(r, v100)

		n := a.vRegIDToNode[100]
		require.Nil(t, n.copyFromVReg)
		require.Nil(t, n.copyToVReg)
		require.Equal(t, RealRegInvalid, n.copyFromReal)
		require.Equal(t, r.RealReg(), n.copyToReal)
	})
	t.Run("virtual/virtual", func(t *testing.T) {
		a := NewAllocator(&RegisterInfo{})
		v100, v200 := VReg(100), VReg(200)
		a.recordCopyRelation(v200, v100)

		n100, n200 := a.vRegIDToNode[100], a.vRegIDToNode[200]
		require.Nil(t, n100.copyFromVReg)
		require.Nil(t, n200.copyToVReg)
		require.Equal(t, n200, n100.copyToVReg)
		require.Equal(t, n200.copyFromVReg, n100)
		require.Equal(t, RealRegInvalid, n100.copyFromReal)
		require.Equal(t, RealRegInvalid, n100.copyToReal)
		require.Equal(t, RealRegInvalid, n200.copyFromReal)
		require.Equal(t, RealRegInvalid, n200.copyToReal)
	})
}

func initMapInInfo(info *blockInfo) {
	if info.liveIns == nil {
		info.liveIns = make(map[VReg]struct{})
	}
	if info.liveOuts == nil {
		info.liveOuts = make(map[VReg]struct{})
	}
	if info.defs == nil {
		info.defs = make(map[VReg]programCounter)
	}
	if info.kills == nil {
		info.kills = make(map[VReg]programCounter)
	}
	if info.lastUses == nil {
		info.lastUses = make(map[VReg]programCounter)
	}
}

func TestNode_assignedRealReg(t *testing.T) {
	require.Equal(t, RealRegInvalid, (&node{}).assignedRealReg())
	require.Equal(t, RealReg(100), (&node{r: 100}).assignedRealReg())
	require.Equal(t, RealReg(200), (&node{v: VReg(1).SetRealReg(200)}).assignedRealReg())
}
