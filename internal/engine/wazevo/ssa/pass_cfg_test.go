package ssa

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestBuilder_passCalculateImmediateDominators(t *testing.T) {
	for _, tc := range []struct {
		name     string
		edges    edgesCase
		expDoms  map[BasicBlockID]BasicBlockID
		expLoops map[BasicBlockID]struct{}
	}{
		{
			name: "linear",
			// 0 -> 1 -> 2 -> 3 -> 4
			edges: edgesCase{
				0: {1},
				1: {2},
				2: {3},
				3: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
				4: 3,
			},
		},
		{
			name: "diamond",
			//  0
			// / \
			// 1   2
			// \ /
			//  3
			edges: edgesCase{
				0: {1, 2},
				1: {3},
				2: {3},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 0,
			},
		},
		{
			name: "merge",
			// 0 -> 1 -> 3
			// |         ^
			// v         |
			// 2 ---------
			edges: edgesCase{
				0: {1, 2},
				1: {3},
				2: {3},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 0,
			},
		},
		{
			name: "branch",
			//  0
			// / \
			// 1   2
			edges: edgesCase{
				0: {1, 2},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
			},
		},
		{
			name: "loop",
			// 0 -> 1 -> 2
			//      ^    |
			//      |    v
			//      |--- 3
			edges: edgesCase{
				0: {1},
				1: {2},
				2: {3},
				3: {1},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
		},
		{
			name: "larger diamond",
			//     0
			//   / | \
			//  1  2  3
			//   \ | /
			//     4
			edges: edgesCase{
				0: {1, 2, 3},
				1: {4},
				2: {4},
				3: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 0,
				4: 0,
			},
		},
		{
			name: "two independent branches",
			//  0
			// / \
			// 1   2
			// |   |
			// 3   4
			edges: edgesCase{
				0: {1, 2},
				1: {3},
				2: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 1,
				4: 2,
			},
		},
		{
			name: "branch",
			// 0 -> 1 -> 2
			//     |    |
			//     v    v
			//     3 <- 4
			edges: edgesCase{
				0: {1},
				1: {2, 3},
				2: {4},
				4: {3},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 1,
				4: 2,
			},
		},
		{
			name: "branches with merge",
			//   0
			// /  \
			// 1   2
			// \   /
			// 3 > 4
			edges: edgesCase{
				0: {1, 2},
				1: {3},
				2: {4},
				3: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 1,
				4: 0,
			},
		},
		{
			name: "cross branches",
			//   0
			//  / \
			// 1   2
			// |\ /|
			// | X |
			// |/ \|
			// 3   4
			edges: edgesCase{
				0: {1, 2},
				1: {3, 4},
				2: {3, 4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 0,
				4: 0,
			},
		},
		{
			// Loop with multiple entries are not loops in the strict sense.
			// See the comment on basicBlock.loopHeader.
			// Note that WebAssembly program won't produce such CFGs. TODO: proof!
			name: "nested loops with multiple entries",
			//     0
			//    / \
			//   v   v
			//   1 <> 2
			//   ^    |
			//   |    v
			//   4 <- 3
			edges: edgesCase{
				0: {1, 2},
				1: {2},
				2: {1, 3},
				3: {4},
				4: {1},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 0,
				3: 2,
				4: 3,
			},
		},
		{
			name: "two intersecting loops",
			//   0
			//   v
			//   1 --> 2 --> 3
			//   ^     |     |
			//   v     v     v
			//   4 <-- 5 <-- 6
			edges: edgesCase{
				0: {1},
				1: {2, 4},
				2: {3, 5},
				3: {6},
				4: {1},
				5: {4},
				6: {5},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
				4: 1,
				5: 2,
				6: 3,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
		},
		{
			name: "loop back edges",
			//     0
			//     v
			//     1 --> 2 --> 3 --> 4
			//     ^           |     |
			//     v           v     v
			//     8 <-------- 6 <-- 5
			edges: edgesCase{
				0: {1},
				1: {2, 8},
				2: {3},
				3: {4, 6},
				4: {5},
				5: {6},
				6: {8},
				8: {1},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
				4: 3,
				5: 4,
				6: 3,
				8: 1,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
		},
		{
			name: "multiple independent paths",
			//   0
			//   v
			//   1 --> 2 --> 3 --> 4 --> 5
			//   |           ^     ^
			//   v           |     |
			//   6 --> 7 --> 8 --> 9
			edges: edgesCase{
				0: {1},
				1: {2, 6},
				2: {3},
				3: {4},
				4: {5},
				6: {7},
				7: {8},
				8: {3, 9},
				9: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 1,
				4: 1,
				5: 4,
				6: 1,
				7: 6,
				8: 7,
				9: 8,
			},
		},
		{
			name: "double back edges",
			//     0
			//     v
			//     1 --> 2 --> 3 --> 4 -> 5
			//     ^                 |
			//     v                 v
			//     7 <--------------- 6
			edges: edgesCase{
				0: {1},
				1: {2, 7},
				2: {3},
				3: {4},
				4: {5, 6},
				6: {7},
				7: {1},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
				4: 3,
				5: 4,
				6: 4,
				7: 1,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
		},
		{
			name: "double nested loops with branches",
			//     0 --> 1 --> 2 --> 3 --> 4 --> 5 --> 6
			//          ^     |            |     |
			//          v     v            v     |
			//          9 <-- 8 <--------- 7 <---|
			edges: edgesCase{
				0: {1},
				1: {2, 9},
				2: {3, 8},
				3: {4},
				4: {5, 7},
				5: {6, 7},
				7: {8},
				8: {9},
				9: {1},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 2,
				4: 3,
				5: 4,
				6: 5,
				7: 4,
				8: 2,
				9: 1,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
		},
		{
			name: "split paths with a loop",
			//       0
			//       v
			//       1
			//      / \
			//     v   v
			//     2<--3
			//     ^   |
			//     |   v
			//     6<--4
			//     |
			//     v
			//     5
			edges: edgesCase{
				0: {1},
				1: {2, 3},
				3: {2, 4},
				4: {6},
				6: {2, 5},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 1,
				4: 3,
				5: 6,
				6: 4,
			},
		},
		{
			name: "multiple exits with a loop",
			//     0
			//     v
			//     1
			//    / \
			//   v   v
			//   2<--3
			//   |
			//   v
			//   4<->5
			//   |
			//   v
			//   6
			edges: edgesCase{
				0: {1},
				1: {2, 3},
				2: {4},
				3: {2},
				4: {5, 6},
				5: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 1,
				4: 2,
				5: 4,
				6: 4,
			},
			expLoops: map[BasicBlockID]struct{}{4: {}},
		},
		{
			name: "parallel loops with merge",
			//       0
			//       v
			//       1
			//      / \
			//     v   v
			//     3<--2
			//     |
			//     v
			//     4<->5
			//     |   |
			//     v   v
			//     7<->6
			edges: edgesCase{
				0: {1},
				1: {2, 3},
				2: {3},
				3: {4},
				4: {5, 7},
				5: {4, 6},
				6: {7},
				7: {6},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 1,
				4: 3,
				5: 4,
				6: 4,
				7: 4,
			},
			expLoops: map[BasicBlockID]struct{}{4: {}},
		},
		{
			name: "two independent loops",
			//      0
			//      |
			//      v
			//      1 --> 2 --> 3
			//      ^           |
			//      v           v
			//      4 <---------5
			//      |
			//      v
			//      6 --> 7 --> 8
			//      ^           |
			//      v           v
			//      9 <---------10
			edges: map[BasicBlockID][]BasicBlockID{
				0:  {1},
				1:  {2, 4},
				2:  {3},
				3:  {5},
				4:  {1, 6},
				5:  {4},
				6:  {7, 9},
				7:  {8},
				8:  {10},
				9:  {6},
				10: {9},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1:  0,
				2:  1,
				3:  2,
				4:  1,
				5:  3,
				6:  4,
				7:  6,
				8:  7,
				9:  6,
				10: 8,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}, 6: {}},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := constructGraphFromEdges(tc.edges)
			passCalculateImmediateDominators(b)

			for blockID, expDomID := range tc.expDoms {
				expBlock := b.basicBlocksPool.View(int(expDomID))
				require.Equal(t, expBlock, b.dominators[blockID],
					"block %d expecting %d, but got %s", blockID, expDomID, b.dominators[blockID])
			}

			for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
				_, expLoop := tc.expLoops[blk.id]
				require.Equal(t, expLoop, blk.loopHeader, blk.String())
			}
		})
	}
}
