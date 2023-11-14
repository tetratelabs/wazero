package ssa

import "sort"

// edgesCase is a map from BasicBlockID to its successors.
type edgesCase map[BasicBlockID][]BasicBlockID

// constructGraphFromEdges constructs a graph from edgesCase.
// This comes in handy when we want to test the CFG related passes.
func constructGraphFromEdges(edges edgesCase) (b *builder) {
	b = NewBuilder().(*builder)

	// Collect edges.
	var maxID BasicBlockID
	var pairs [][2]BasicBlockID
	for fromID, toIDs := range edges {
		if maxID < fromID {
			maxID = fromID
		}
		for _, toID := range toIDs {
			pairs = append(pairs, [2]BasicBlockID{fromID, toID})
			if maxID < toID {
				maxID = toID
			}
		}
	}

	// Allocate blocks.
	blocks := make(map[BasicBlockID]*basicBlock, maxID+1)
	for i := 0; i < int(maxID)+1; i++ {
		blk := b.AllocateBasicBlock().(*basicBlock)
		blocks[blk.id] = blk
	}

	// To have a consistent behavior in test, we sort the pairs by fromID.
	sort.Slice(pairs, func(i, j int) bool {
		xf, yf := pairs[i][0], pairs[j][0]
		return xf < yf
	})

	// Add edges.
	for _, p := range pairs {
		from, to := blocks[p[0]], blocks[p[1]]
		to.addPred(from, &Instruction{})
	}
	return
}
