package regalloc

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func makeVRegIDMinSet[T any](vregs map[VReg]T) (min VRegIDMinSet) {
	for v := range vregs {
		min.Observe(v)
	}
	return min
}

func makeVRegTable(vregs map[VReg]programCounter) (table VRegTable) {
	table.Reset(makeVRegIDMinSet(vregs))
	for v, p := range vregs {
		table.Insert(v, p)
	}
	return table
}

func TestAllocator_livenessAnalysis(t *testing.T) {
	const realRegID, realRegID2 = 50, 100
	realReg, realReg2 := FromRealReg(realRegID, RegTypeInt), FromRealReg(realRegID2, RegTypeInt)
	phiVReg := VReg(12345).SetRegType(RegTypeInt)
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
					lastUses: makeVRegTable(map[VReg]programCounter{1: pcStride + pcUseOffset}),
				},
			},
		},
		{
			name: "single block with real reg",
			setup: func() Function {
				realVReg := FromRealReg(10, RegTypeInt)
				param := VReg(1)
				ret := VReg(2)
				blk := newMockBlock(0,
					newMockInstr().def(param).use(realVReg),
					newMockInstr().def(ret).use(param, param),
					newMockInstr().def(realVReg).use(ret),
				).entry()
				blk.blockParam(param)
				return newMockFunction(blk)
			},
			exp: map[int]*blockInfo{
				0: {
					defs: map[VReg]programCounter{1: 1, 2: pcDefOffset + pcStride},
					lastUses: makeVRegTable(map[VReg]programCounter{
						1: pcStride + pcUseOffset,
						2: pcStride*2 + pcUseOffset,
					}),
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
					lastUses: makeVRegTable(map[VReg]programCounter{
						1000: pcStride + pcUseOffset,
						1:    pcStride*2 + pcUseOffset,
						2:    pcStride*2 + pcUseOffset,
					}),
					liveOuts: map[VReg]struct{}{3: {}},
				},
				1: {
					liveIns:  map[VReg]struct{}{3: {}},
					liveOuts: map[VReg]struct{}{3: {}, 4: {}, 5: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{
						3: pcStride + pcUseOffset,
					}),
					defs: map[VReg]programCounter{
						4: pcStride + pcDefOffset,
						5: pcStride + pcDefOffset,
					},
				},
				2: {
					liveIns:  map[VReg]struct{}{3: {}, 4: {}, 5: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{3: pcUseOffset, 4: pcUseOffset, 5: pcUseOffset}),
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
					lastUses: makeVRegTable(nil),
				},
				1: {
					liveIns:  map[VReg]struct{}{1000: {}, 1: {}},
					liveOuts: map[VReg]struct{}{1000: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{1: pcUseOffset}),
				},
				2: {
					liveIns:  map[VReg]struct{}{1000: {}, 2: {}},
					liveOuts: map[VReg]struct{}{1000: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{2: pcUseOffset}),
				},
				3: {
					liveIns:  map[VReg]struct{}{1000: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{1000: pcUseOffset}),
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
					lastUses: makeVRegTable(nil),
				},
				1: {
					liveIns:  map[VReg]struct{}{2000: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{2000: pcUseOffset}),
				},
				2: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
					lastUses: makeVRegTable(nil),
				},
				3: {
					liveIns:  map[VReg]struct{}{1000: {}, 3000: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 3000: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{1000: pcUseOffset}),
				},
				4: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 3000: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{phiVReg: pcUseOffset, 3000: pcUseOffset}),
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
				b1.blockParam(phiVReg)
				b2 := newMockBlock(2,
					newMockInstr().def(100).use(phiVReg, 9999),
				)
				b3 := newMockBlock(3,
					newMockInstr().def(54321),
					newMockInstr().use(100),
				)
				b4 := newMockBlock(4,
					newMockInstr().def(phiVReg).use(54321).
						// Make sure this is the PHI defining instruction.
						asCopy(),
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
				b1.loop(b2, b3, b4, b5)
				f := newMockFunction(b0, b1, b2, b3, b4, b5)
				f.loopNestingForestRoots(b1)
				return f
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
					lastUses: makeVRegTable(map[VReg]programCounter{
						1: pcStride + pcUseOffset,
					}),
				},
				1: {
					liveIns:  map[VReg]struct{}{phiVReg: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}, 9999: {}},
					defs:     map[VReg]programCounter{phiVReg: 0, 9999: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{}),
				},
				2: {
					liveIns:  map[VReg]struct{}{phiVReg: {}, 9999: {}},
					liveOuts: map[VReg]struct{}{100: {}},
					defs:     map[VReg]programCounter{100: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{phiVReg: pcUseOffset, 9999: pcUseOffset}),
				},
				3: {
					liveIns:  map[VReg]struct{}{100: {}},
					liveOuts: map[VReg]struct{}{54321: {}},
					defs:     map[VReg]programCounter{54321: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{100: pcStride + pcUseOffset}),
				},
				4: {
					liveIns:  map[VReg]struct{}{54321: {}},
					liveOuts: map[VReg]struct{}{phiVReg: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{54321: pcUseOffset}),
				},
			},
		},
		{
			name: "multiple pass alive",
			setup: func() Function {
				v := VReg(9999)
				b0 := newMockBlock(0, newMockInstr().def(v)).entry()

				b1, b2, b3, b4, b5, b6 := newMockBlock(1), newMockBlock(2),
					newMockBlock(3, newMockInstr().use(v)),
					newMockBlock(4), newMockBlock(5), newMockBlock(6)

				b1.addPred(b0)
				b4.addPred(b0)
				b2.addPred(b1)
				b5.addPred(b2)
				b2.addPred(b5)
				b6.addPred(b2)
				b3.addPred(b6)
				b3.addPred(b4)
				f := newMockFunction(b0, b1, b2, b4, b5, b6, b3)
				f.loopNestingForestRoots(b2)
				return f
			},
			exp: map[int]*blockInfo{
				0: {
					liveOuts: map[VReg]struct{}{9999: {}},
					defs:     map[VReg]programCounter{9999: pcDefOffset},
					lastUses: makeVRegTable(nil),
				},
				1: {
					liveIns:  map[VReg]struct{}{9999: {}},
					liveOuts: map[VReg]struct{}{9999: {}},
					lastUses: makeVRegTable(nil),
				},
				2: {
					liveIns:  map[VReg]struct{}{9999: {}},
					liveOuts: map[VReg]struct{}{9999: {}},
					lastUses: makeVRegTable(nil),
				},
				3: {
					liveIns:  map[VReg]struct{}{9999: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{9999: pcUseOffset}),
				},
				4: {
					liveIns:  map[VReg]struct{}{9999: {}},
					liveOuts: map[VReg]struct{}{9999: {}},
					lastUses: makeVRegTable(nil),
				},
				5: {lastUses: makeVRegTable(nil)},
				6: {
					liveIns:  map[VReg]struct{}{9999: {}},
					liveOuts: map[VReg]struct{}{9999: {}},
					lastUses: makeVRegTable(nil),
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
				b0 := newMockBlock(0,
					newMockInstr().def(99999),
					newMockInstr().def(phiVReg).use(111).asCopy(),
				).entry()
				b1 := newMockBlock(1, newMockInstr().use(99999))
				b1.blockParam(phiVReg)
				b2 := newMockBlock(2, newMockInstr().def(88888).use(phiVReg, phiVReg))
				b3 := newMockBlock(3, newMockInstr().def(phiVReg).use(88888).asCopy())
				b4 := newMockBlock(4)
				b1.addPred(b0)
				b1.addPred(b2)
				b2.addPred(b1)
				b2.addPred(b3)
				b3.addPred(b2)
				b4.addPred(b3)

				b1.loop(b2)
				b2.loop(b3)
				f := newMockFunction(b0, b1, b2, b3, b4)
				f.loopNestingForestRoots(b1)
				return f
			},
			exp: map[int]*blockInfo{
				0: {
					defs:     map[VReg]programCounter{99999: pcDefOffset, phiVReg: pcStride + pcDefOffset},
					liveOuts: map[VReg]struct{}{99999: {}, phiVReg: {}},
					liveIns:  map[VReg]struct{}{111: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{111: pcStride + pcUseOffset}),
				},
				1: {
					defs:     map[VReg]programCounter{phiVReg: 0},
					liveIns:  map[VReg]struct{}{99999: {}, phiVReg: {}},
					liveOuts: map[VReg]struct{}{99999: {}, phiVReg: {}},
					lastUses: makeVRegTable(map[VReg]programCounter{99999: pcUseOffset}),
				},
				2: {
					liveIns:  map[VReg]struct{}{99999: {}, phiVReg: {}},
					liveOuts: map[VReg]struct{}{99999: {}, 88888: {}, phiVReg: {}},
					defs:     map[VReg]programCounter{88888: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{phiVReg: pcUseOffset}),
				},
				3: {
					liveIns:  map[VReg]struct{}{99999: {}, phiVReg: {}, 88888: {}},
					liveOuts: map[VReg]struct{}{99999: {}, phiVReg: {}},
					defs:     map[VReg]programCounter{phiVReg: pcDefOffset},
					lastUses: makeVRegTable(map[VReg]programCounter{88888: pcUseOffset}),
				},
				4: {
					lastUses: makeVRegTable(nil),
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := tc.setup()
			a := NewAllocator(&RegisterInfo{
				RealRegName: func(r RealReg) string {
					return fmt.Sprintf("r%d", r)
				},
			})
			a.livenessAnalysis(f)
			for blockID := range a.blockInfos {
				t.Run(fmt.Sprintf("block_id=%d", blockID), func(t *testing.T) {
					actual := a.blockInfos[blockID]
					exp := tc.exp[blockID]
					initMapInInfo(exp)
					fmt.Printf("\n[exp for block[%d]]\n%v\n[actual for block[%d]]\n%v\n",
						blockID, exp.Format(), blockID, actual.Format())

					require.Equal(t, exp.liveOuts, actual.liveOuts, "live outs")
					require.Equal(t, exp.liveIns, actual.liveIns, "live ins")
					require.Equal(t, exp.defs, actual.defs, "defs")
					require.Equal(t, exp.lastUses, actual.lastUses, "last uses")
				})
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
}

func TestNode_assignedRealReg(t *testing.T) {
	require.Equal(t, RealRegInvalid, (&node{}).assignedRealReg())
	require.Equal(t, RealReg(100), (&node{r: 100}).assignedRealReg())
	require.Equal(t, RealReg(200), (&node{v: VReg(1).SetRealReg(200)}).assignedRealReg())
}

func TestAllocator_finalizeEdges(t *testing.T) {
	for i, tc := range []struct {
		edges        [][2]nodeID
		expEdgeNum   int
		expDegrees   map[nodeID]int
		expEdgeIndex map[nodeID][2]int // [Begin, End]
	}{
		{
			edges:      [][2]nodeID{{0, 1}},
			expEdgeNum: 2,
			expDegrees: map[nodeID]int{
				0: 1, 1: 1,
				2: 0,
				3: 0,
				4: 0,
			},
			expEdgeIndex: map[nodeID][2]int{
				0: {0, 0},
				1: {1, 1},
				2: {0, -1},
				3: {0, -1},
				4: {0, -1},
			},
		},
		{
			edges:      [][2]nodeID{{0, 1}, {0, 2}},
			expEdgeNum: 4,
			expDegrees: map[nodeID]int{
				0: 2, 1: 1, 2: 1,
				3: 0,
				4: 0,
			},
			expEdgeIndex: map[nodeID][2]int{
				0: {0, 1},
				1: {2, 2},
				2: {3, 3},
				3: {0, -1},
				4: {0, -1},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			a := NewAllocator(&RegisterInfo{})
			for i := 0; i < 5; i++ {
				a.allocateNode()
			}
			for _, edge := range tc.edges {
				n := a.nodePool.View(int(edge[0]))
				m := a.nodePool.View(int(edge[1]))
				a.maybeAddEdge(n, m)
			}
			a.finalizeEdges()
			require.Equal(t, tc.expEdgeNum, len(a.edges))
			for nID, expDegree := range tc.expDegrees {
				t.Run(fmt.Sprintf("node_id=%d", nID), func(t *testing.T) {
					n := a.nodePool.View(int(nID))
					require.Equal(t, expDegree, n.degree)
					expEdgeIndex := tc.expEdgeIndex[nID]
					require.Equal(t, expEdgeIndex[0], n.edgeIdxBegin)
					require.Equal(t, expEdgeIndex[1], n.edgeIdxEnd)
				})
			}
		})
	}
}
