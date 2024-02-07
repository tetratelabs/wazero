package regalloc

import (
	"fmt"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAllocator_livenessAnalysis(t *testing.T) {
	const realRegID, realRegID2 = 50, 100
	realReg, realReg2 := FromRealReg(realRegID, RegTypeInt), FromRealReg(realRegID2, RegTypeInt)
	phiVReg := VReg(12345).SetRegType(RegTypeInt)
	for _, tc := range []struct {
		name  string
		setup func() Function
		exp   map[int]*blockLivenessData
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
			exp: map[int]*blockLivenessData{
				0: {},
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
			exp: map[int]*blockLivenessData{
				0: {},
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
			exp: map[int]*blockLivenessData{
				0: {},
				1: {
					liveIns: []VReg{3},
				},
				2: {liveIns: []VReg{3, 4, 5}},
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
			exp: map[int]*blockLivenessData{
				0: {},
				1: {liveIns: []VReg{1000, 1}},
				2: {
					liveIns: []VReg{1000, 2},
				},
				3: {
					liveIns: []VReg{1000},
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
			exp: map[int]*blockLivenessData{
				0: {},
				1: {
					liveIns: []VReg{2000, 3000},
				},
				2: {
					liveIns: []VReg{phiVReg, 3000},
				},
				3: {
					liveIns: []VReg{1000, 3000},
				},
				4: {
					liveIns: []VReg{phiVReg, 3000},
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
					5, newMockInstr().use(54321),
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
			exp: map[int]*blockLivenessData{
				0: {
					liveIns: []VReg{},
				},
				1: {
					liveIns: []VReg{phiVReg},
				},
				2: {
					liveIns: []VReg{phiVReg, 9999},
				},
				3: {
					liveIns: []VReg{100},
				},
				4: {
					liveIns: []VReg{54321},
				},
				5: {liveIns: []VReg{54321}},
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
			exp: map[int]*blockLivenessData{
				0: {},
				1: {
					liveIns: []VReg{9999},
				},
				2: {
					liveIns: []VReg{9999},
				},
				3: {
					liveIns: []VReg{9999},
				},
				4: {
					liveIns: []VReg{9999},
				},
				5: {},
				6: {
					liveIns: []VReg{9999},
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
			exp: map[int]*blockLivenessData{
				0: {
					liveIns: []VReg{111},
				},
				1: {
					liveIns: []VReg{99999, phiVReg},
				},
				2: {
					liveIns: []VReg{99999, phiVReg},
				},
				3: {
					liveIns: []VReg{99999, phiVReg, 88888},
				},
				4: {},
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
			for blockID := 0; blockID <= a.blockLivenessData.MaxIDEncountered(); blockID++ {
				actual := a.blockLivenessData.Get(blockID)
				if actual == nil {
					continue
				}
				t.Run(fmt.Sprintf("block_id=%d", blockID), func(t *testing.T) {
					exp := tc.exp[blockID]
					if len(exp.liveIns) == 0 {
						require.Nil(t, actual.liveIns, "live ins")
					} else {
						sort.Slice(actual.liveIns, func(i, j int) bool {
							return actual.liveIns[i] < actual.liveIns[j]
						})
						sort.Slice(exp.liveIns, func(i, j int) bool {
							return exp.liveIns[i] < exp.liveIns[j]
						})
						require.Equal(t, exp.liveIns, actual.liveIns, "live ins")
					}
				})
			}
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
}
