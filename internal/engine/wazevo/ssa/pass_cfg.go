package ssa

// passCalculateImmediateDominators calculates immediate dominators for each basic block.
// The result is stored in b.dominators. This make it possible for the following passes to
// use builder.isDominatedBy to check if a block is dominated by another block.
//
// At the last of pass, this function also does the loop detection and sets the basicBlock.loop flag.
func passCalculateImmediateDominators(b *builder) {
	reversePostOrder := b.reversePostOrderedBasicBlocks[:0]
	exploreStack := b.blkStack[:0]
	b.clearBlkVisited()

	entryBlk := b.entryBlk()

	// Store the reverse postorder from the entrypoint into reversePostOrder slice.
	// This calculation of reverse postorder is not described in the paper,
	// so we use heuristic to calculate it so that we could potentially handle arbitrary
	// complex CFGs under the assumption that success is sorted in program's natural order.
	// That means blk.success[i] always appears before blk.success[i+1] in the source program,
	// which is a reasonable assumption as long as SSA Builder is properly used.
	//
	// First we push blocks in postorder iteratively visit successors of the entry block.
	exploreStack = append(exploreStack, entryBlk)
	const visitStateUnseen, visitStateSeen, visitStateDone = 0, 1, 2
	b.blkVisited[entryBlk] = visitStateSeen
	for len(exploreStack) > 0 {
		tail := len(exploreStack) - 1
		blk := exploreStack[tail]
		exploreStack = exploreStack[:tail]
		switch b.blkVisited[blk] {
		case visitStateUnseen:
			// This is likely a bug in the frontend.
			panic("BUG: unsupported CFG")
		case visitStateSeen:
			// This is the first time to pop this block, and we have to see the successors first.
			// So push this block again to the stack.
			exploreStack = append(exploreStack, blk)
			// And push the successors to the stack if necessary.
			for _, succ := range blk.success {
				if succ.ReturnBlock() || succ.invalid {
					continue
				}
				if b.blkVisited[succ] == visitStateUnseen {
					b.blkVisited[succ] = visitStateSeen
					exploreStack = append(exploreStack, succ)
				}
			}
			// Finally, we could pop this block once we pop all of its successors.
			b.blkVisited[blk] = visitStateDone
		case visitStateDone:
			// Note: at this point we push blk in postorder despite its name.
			reversePostOrder = append(reversePostOrder, blk)
		}
	}
	// At this point, reversePostOrder has postorder actually, so we reverse it.
	for i := len(reversePostOrder)/2 - 1; i >= 0; i-- {
		j := len(reversePostOrder) - 1 - i
		reversePostOrder[i], reversePostOrder[j] = reversePostOrder[j], reversePostOrder[i]
	}

	for i, blk := range reversePostOrder {
		blk.reversePostOrder = i
	}

	// Reuse the dominators slice if possible from the previous computation of function.
	b.dominators = b.dominators[:cap(b.dominators)]
	if len(b.dominators) < b.basicBlocksPool.Allocated() {
		// Generously reserve space in the slice because the slice will be reused future allocation.
		b.dominators = append(b.dominators, make([]*basicBlock, b.basicBlocksPool.Allocated())...)
	}
	calculateDominators(reversePostOrder, b.dominators)

	// Reuse the slices for the future use.
	b.blkStack = exploreStack

	// For the following passes.
	b.reversePostOrderedBasicBlocks = reversePostOrder

	// Ready to detect loops!
	subPassLoopDetection(b)
}

// calculateDominators calculates the immediate dominator of each node in the CFG, and store the result in `doms`.
// The algorithm is based on the one described in the paper "A Simple, Fast Dominance Algorithm"
// https://www.cs.rice.edu/~keith/EMBED/dom.pdf which is a faster/simple alternative to the well known Lengauer-Tarjan algorithm.
//
// The following code almost matches the pseudocode in the paper with one exception (see the code comment below).
//
// The result slice `doms` must be pre-allocated with the size larger than the size of dfsBlocks.
func calculateDominators(reversePostOrderedBlks []*basicBlock, doms []*basicBlock) {
	entry, reversePostOrderedBlks := reversePostOrderedBlks[0], reversePostOrderedBlks[1: /* skips entry point */]
	for _, blk := range reversePostOrderedBlks {
		doms[blk.id] = nil
	}
	doms[entry.id] = entry

	changed := true
	for changed {
		changed = false
		for _, blk := range reversePostOrderedBlks {
			var u *basicBlock
			for i := range blk.preds {
				pred := blk.preds[i].blk
				// Skip if this pred is not reachable yet. Note that this is not described in the paper,
				// but it is necessary to handle nested loops etc.
				if doms[pred.id] == nil {
					continue
				}

				if u == nil {
					u = pred
					continue
				} else {
					u = intersect(doms, u, pred)
				}
			}
			if doms[blk.id] != u {
				doms[blk.id] = u
				changed = true
			}
		}
	}
}

// intersect returns the common dominator of blk1 and blk2.
//
// This is the `intersect` function in the paper.
func intersect(doms []*basicBlock, blk1 *basicBlock, blk2 *basicBlock) *basicBlock {
	finger1, finger2 := blk1, blk2
	for finger1 != finger2 {
		// Move the 'finger1' upwards to its immediate dominator.
		for finger1.reversePostOrder > finger2.reversePostOrder {
			finger1 = doms[finger1.id]
		}
		// Move the 'finger2' upwards to its immediate dominator.
		for finger2.reversePostOrder > finger1.reversePostOrder {
			finger2 = doms[finger2.id]
		}
	}
	return finger1
}

// subPassLoopDetection detects loops in the function using the immediate dominators.
//
// This is run at the last of passCalculateImmediateDominators.
func subPassLoopDetection(b *builder) {
	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for i := range blk.preds {
			pred := blk.preds[i].blk
			if pred.invalid {
				continue
			}
			if b.isDominatedBy(pred, blk) {
				blk.loopHeader = true
			}
		}
	}
}
