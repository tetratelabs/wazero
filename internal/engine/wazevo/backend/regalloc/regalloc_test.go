package regalloc

import (
	"fmt"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

type (
	_allocator = Allocator[*mockInstr, *mockBlock, *mockFunction]
	_state     = state[*mockInstr, *mockBlock, *mockFunction]
	_vrState   = vrState[*mockInstr, *mockBlock, *mockFunction]
)

var (
	_newAllocator = NewAllocator[*mockInstr, *mockBlock, *mockFunction]
	_resetVrState = resetVrState[*mockInstr, *mockBlock, *mockFunction]
)

func TestAllocator_livenessAnalysis(t *testing.T) {
	const realRegID, realRegID2 = 50, 100
	realReg, realReg2 := FromRealReg(realRegID, RegTypeInt), FromRealReg(realRegID2, RegTypeInt)
	phiVReg := VReg(12345).SetRegType(RegTypeInt)

	type exp struct {
		liveIns []VRegID
	}

	for _, tc := range []struct {
		name  string
		setup func() *mockFunction
		exps  map[int]*exp
	}{
		{
			name: "single block",
			setup: func() *mockFunction {
				return newMockFunction(
					newMockBlock(0,
						newMockInstr().def(1),
						newMockInstr().use(1).def(2),
					).entry(),
				)
			},
			exps: map[int]*exp{
				0: {},
			},
		},
		{
			name: "single block with real reg",
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {},
			},
		},
		{
			name: "straight",
			// b0 -> b1 -> b2
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {},
				1: {
					liveIns: []VRegID{3},
				},
				2: {liveIns: []VRegID{3, 4, 5}},
			},
		},
		{
			name: "diamond",
			//  0   v1000<-, v1<-, v2<-
			// / \
			// 1   2
			// \ /
			//  3
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {},
				1: {liveIns: []VRegID{1000, 1}},
				2: {
					liveIns: []VRegID{1000, 2},
				},
				3: {
					liveIns: []VRegID{1000},
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
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {},
				1: {
					liveIns: []VRegID{2000, 3000},
				},
				2: {
					liveIns: []VRegID{phiVReg.ID(), 3000},
				},
				3: {
					liveIns: []VRegID{1000, 3000},
				},
				4: {
					liveIns: []VRegID{phiVReg.ID(), 3000},
				},
			},
		},

		{
			name: "loop",
			// 0 -> 1 -> 2
			//      ^    |
			//      |    v
			//      4 <- 3 -> 5
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {
					liveIns: []VRegID{},
				},
				1: {
					liveIns: []VRegID{phiVReg.ID()},
				},
				2: {
					liveIns: []VRegID{phiVReg.ID(), 9999},
				},
				3: {
					liveIns: []VRegID{100},
				},
				4: {
					liveIns: []VRegID{54321},
				},
				5: {liveIns: []VRegID{54321}},
			},
		},
		{
			name: "multiple pass alive",
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {},
				1: {
					liveIns: []VRegID{9999},
				},
				2: {
					liveIns: []VRegID{9999},
				},
				3: {
					liveIns: []VRegID{9999},
				},
				4: {
					liveIns: []VRegID{9999},
				},
				5: {},
				6: {
					liveIns: []VRegID{9999},
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
			setup: func() *mockFunction {
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
			exps: map[int]*exp{
				0: {
					liveIns: []VRegID{111},
				},
				1: {
					liveIns: []VRegID{99999, phiVReg.ID()},
				},
				2: {
					liveIns: []VRegID{99999, phiVReg.ID()},
				},
				3: {
					liveIns: []VRegID{99999, phiVReg.ID(), 88888},
				},
				4: {},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := tc.setup()
			a := _newAllocator(&RegisterInfo{
				RealRegName: func(r RealReg) string {
					return fmt.Sprintf("r%d", r)
				},
			})
			a.livenessAnalysis(f)
			for blockID := 0; blockID <= a.blockStates.MaxIDEncountered(); blockID++ {
				actual := a.blockStates.Get(blockID)
				if actual == nil {
					continue
				}
				t.Run(fmt.Sprintf("block_id=%d", blockID), func(t *testing.T) {
					exp := tc.exps[blockID]
					if len(exp.liveIns) == 0 {
						require.Nil(t, actual.liveIns, "live ins")
					} else {
						var actuals []VRegID
						for _, s := range actual.liveIns {
							actuals = append(actuals, s.v.ID())
						}
						sort.Slice(actuals, func(i, j int) bool {
							return actuals[i] < actuals[j]
						})
						sort.Slice(exp.liveIns, func(i, j int) bool {
							return exp.liveIns[i] < exp.liveIns[j]
						})
						require.Equal(t, exp.liveIns, actuals, "live ins")
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
	a := _newAllocator(&RegisterInfo{})
	a.livenessAnalysis(f)
}

func Test_findOrSpillAllocatable_prefersSpill(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		a := &_allocator{}
		s := &_state{regsInUse: newRegInUseSet[*mockInstr, *mockBlock, *mockFunction]()}
		s.regsInUse.add(RealReg(1), &_vrState{v: VReg(2222222)})
		got := a.findOrSpillAllocatable(s, []RealReg{3}, 0, 3)
		require.Equal(t, RealReg(3), got)
	})
	t.Run("preferred but in use", func(t *testing.T) {
		a := &_allocator{}
		s := &_state{vrStates: wazevoapi.NewIDedPool[_vrState](_resetVrState)}
		s.regsInUse.add(RealReg(3), &_vrState{v: VReg(1).SetRealReg(3)})
		got := a.findOrSpillAllocatable(s, []RealReg{3, 4}, 0, 3)
		require.Equal(t, RealReg(4), got)
	})
	t.Run("preferred but forbidden", func(t *testing.T) {
		a := &_allocator{}
		s := &_state{vrStates: wazevoapi.NewIDedPool[_vrState](_resetVrState)}
		got := a.findOrSpillAllocatable(s, []RealReg{3, 4}, RegSet(0).add(3), 3)
		require.Equal(t, RealReg(4), got)
	})
}
