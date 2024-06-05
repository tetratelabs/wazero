package ssa

import (
	"sort"
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
		{
			name: "merge after loop",
			edges: edgesCase{
				0: {3, 1},
				1: {2},
				2: {1, 3},
				3: {4},
			},
			expDoms: map[BasicBlockID]BasicBlockID{
				1: 0,
				2: 1,
				3: 0,
				4: 3,
			},
			expLoops: map[BasicBlockID]struct{}{1: {}},
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

func TestBuildLoopNestingForest(t *testing.T) {
	type expLoopNestingForest struct {
		roots    []BasicBlockID
		children map[BasicBlockID][]BasicBlockID
	}

	for _, tc := range []struct {
		name   string
		edges  edgesCase
		expLNF expLoopNestingForest
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
			expLNF: expLoopNestingForest{
				roots: []BasicBlockID{1},
				children: map[BasicBlockID][]BasicBlockID{
					1: {2, 3},
				},
			},
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
			expLNF: expLoopNestingForest{
				roots: []BasicBlockID{1},
				children: map[BasicBlockID][]BasicBlockID{
					1: {2, 3, 4, 5, 6},
					6: {7, 8, 9, 10},
				},
			},
		},
		{
			//
			//                  +-----+
			//                  |     |
			//                  v     |
			//    0 ---> 1 ---> 2 --> 3 ---> 4
			//           ^      |
			//           |      |
			//           +------+
			//
			name: "Fig. 9.2", // in "SSA-based Compiler Design".
			edges: map[BasicBlockID][]BasicBlockID{
				0: {1},
				1: {2},
				2: {1, 3},
				3: {2, 4},
			},
			expLNF: expLoopNestingForest{
				roots: []BasicBlockID{1},
				children: map[BasicBlockID][]BasicBlockID{
					1: {2},
					2: {3, 4},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := constructGraphFromEdges(tc.edges)
			// buildLoopNestingForest requires passCalculateImmediateDominators to be done.
			passCalculateImmediateDominators(b)
			passBuildLoopNestingForest(b)

			blocks := map[BasicBlockID]*basicBlock{}
			for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
				blocks[blk.id] = blk
			}

			// Check the result of buildLoopNestingForest.
			var forestRoots []BasicBlockID
			for _, root := range b.loopNestingForestRoots {
				forestRoots = append(forestRoots, root.(*basicBlock).id)
			}
			sort.Slice(forestRoots, func(i, j int) bool {
				return forestRoots[i] < forestRoots[j]
			})
			require.Equal(t, tc.expLNF.roots, forestRoots)

			for expBlkID, blk := range blocks {
				expChildren := tc.expLNF.children[expBlkID]
				var actualChildren []BasicBlockID
				for _, child := range blk.loopNestingForestChildren.View() {
					actualChildren = append(actualChildren, child.(*basicBlock).id)
				}
				sort.Slice(actualChildren, func(i, j int) bool {
					return actualChildren[i] < actualChildren[j]
				})
				require.Equal(t, expChildren, actualChildren, "block %d", expBlkID)
			}
		})
	}
}

func TestDominatorTree(t *testing.T) {
	type lowestCommonAncestorCase struct {
		a, b BasicBlockID
		exp  BasicBlockID
	}

	for _, tc := range []struct {
		name  string
		edges edgesCase
		cases []lowestCommonAncestorCase
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
			cases: []lowestCommonAncestorCase{
				{a: 0, b: 0, exp: 0},
				{a: 0, b: 1, exp: 0},
				{a: 0, b: 2, exp: 0},
				{a: 0, b: 3, exp: 0},
				{a: 0, b: 4, exp: 0},
				{a: 1, b: 1, exp: 1},
				{a: 1, b: 2, exp: 1},
				{a: 1, b: 3, exp: 1},
				{a: 1, b: 4, exp: 1},
				{a: 2, b: 2, exp: 2},
				{a: 2, b: 3, exp: 2},
				{a: 2, b: 4, exp: 2},
				{a: 3, b: 3, exp: 3},
				{a: 3, b: 4, exp: 3},
				{a: 4, b: 4, exp: 4},
			},
		},
		{
			//
			//                  +-----+
			//                  |     |
			//                  v     |
			//    0 ---> 1 ---> 2 --> 3 ---> 4
			//           ^      |
			//           |      |
			//           +------+
			//
			name: "two loops",
			edges: map[BasicBlockID][]BasicBlockID{
				0: {1},
				1: {2},
				2: {1, 3},
				3: {2, 4},
			},
			cases: []lowestCommonAncestorCase{
				{a: 0, b: 0, exp: 0},
				{a: 0, b: 1, exp: 0},
				{a: 0, b: 2, exp: 0},
				{a: 0, b: 3, exp: 0},
				{a: 0, b: 4, exp: 0},
				{a: 1, b: 1, exp: 1},
				{a: 1, b: 2, exp: 1},
				{a: 1, b: 3, exp: 1},
				{a: 1, b: 4, exp: 1},
				{a: 2, b: 2, exp: 2},
				{a: 2, b: 3, exp: 2},
				{a: 2, b: 4, exp: 2},
				{a: 3, b: 3, exp: 3},
				{a: 3, b: 4, exp: 3},
				{a: 4, b: 4, exp: 4},
			},
		},
		{
			name: "binary",
			edges: edgesCase{
				0: {1, 2},
				1: {3, 4},
				2: {5, 6},
				3: {},
				4: {},
				5: {},
				6: {},
			},
			cases: []lowestCommonAncestorCase{
				{a: 3, b: 4, exp: 1},
				{a: 3, b: 5, exp: 0},
				{a: 3, b: 6, exp: 0},
				{a: 4, b: 5, exp: 0},
				{a: 4, b: 6, exp: 0},
				{a: 5, b: 6, exp: 2},
				{a: 3, b: 1, exp: 1},
				{a: 6, b: 2, exp: 2},
			},
		},
		{
			name: "complex tree",
			edges: edgesCase{
				0:  {1, 2},
				1:  {3, 4, 5},
				2:  {6, 7},
				3:  {8, 9},
				4:  {10},
				6:  {11, 12, 13},
				7:  {14},
				12: {15},
			},
			cases: []lowestCommonAncestorCase{
				{a: 8, b: 9, exp: 3},
				{a: 10, b: 5, exp: 1},
				{a: 11, b: 14, exp: 2},
				{a: 15, b: 13, exp: 6},
				{a: 8, b: 10, exp: 1},
				{a: 9, b: 4, exp: 1},
				{a: 15, b: 7, exp: 2},
			},
		},
		{
			name: "complex tree with single and multiple branching",
			edges: edgesCase{
				0:  {1, 2},
				1:  {3},
				2:  {4, 5},
				3:  {6, 7, 8},
				4:  {9, 10},
				5:  {11},
				8:  {12, 13},
				9:  {14, 15},
				10: {16},
				13: {17, 18},
			},
			cases: []lowestCommonAncestorCase{
				{a: 6, b: 7, exp: 3},
				{a: 14, b: 16, exp: 4},
				{a: 17, b: 18, exp: 13},
				{a: 12, b: 18, exp: 8},
				{a: 6, b: 12, exp: 3},
				{a: 11, b: 9, exp: 2},
				{a: 15, b: 11, exp: 2},
				{a: 3, b: 10, exp: 0},
				{a: 7, b: 13, exp: 3},
				{a: 15, b: 16, exp: 4},
				{a: 0, b: 17, exp: 0},
				{a: 1, b: 18, exp: 1},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := constructGraphFromEdges(tc.edges)
			// buildDominatorTree requires passCalculateImmediateDominators to be done.
			passCalculateImmediateDominators(b)
			passBuildDominatorTree(b)

			for _, c := range tc.cases {
				exp := b.sparseTree.findLCA(c.a, c.b)
				require.Equal(t, exp.id, c.exp, "LCA(%d, %d)", c.a, c.b)
			}
		})
	}
}
