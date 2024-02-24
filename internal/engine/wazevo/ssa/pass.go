package ssa

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// RunPasses implements Builder.RunPasses.
//
// The order here matters; some pass depends on the previous ones.
//
// Note that passes suffixed with "Opt" are the optimization passes, meaning that they edit the instructions and blocks
// while the other passes are not, like passEstimateBranchProbabilities does not edit them, but only calculates the additional information.
func (b *builder) RunPasses() {
	b.runPreBlockLayoutPasses()
	b.runBlockLayoutPass()
	b.runPostBlockLayoutPasses()
	b.runFinalizingPasses()
}

func (b *builder) runPreBlockLayoutPasses() {
	passSortSuccessors(b)
	passDeadBlockEliminationOpt(b)
	passRedundantPhiEliminationOpt(b)
	// The result of passCalculateImmediateDominators will be used by various passes below.
	passCalculateImmediateDominators(b)
	passNopInstElimination(b)

	// TODO: implement either conversion of irreducible CFG into reducible one, or irreducible CFG detection where we panic.
	// 	WebAssembly program shouldn't result in irreducible CFG, but we should handle it properly in just in case.
	// 	See FixIrreducible pass in LLVM: https://llvm.org/doxygen/FixIrreducible_8cpp_source.html

	// TODO: implement more optimization passes like:
	// 	block coalescing.
	// 	Copy-propagation.
	// 	Constant folding.
	// 	Common subexpression elimination.
	// 	Arithmetic simplifications.
	// 	and more!

	// passDeadCodeEliminationOpt could be more accurate if we do this after other optimizations.
	passDeadCodeEliminationOpt(b)
	b.donePreBlockLayoutPasses = true
}

func (b *builder) runBlockLayoutPass() {
	if !b.donePreBlockLayoutPasses {
		panic("runBlockLayoutPass must be called after all pre passes are done")
	}
	passLayoutBlocks(b)
	b.doneBlockLayout = true
}

// runPostBlockLayoutPasses runs the post block layout passes. After this point, CFG is somewhat stable,
// but still can be modified before finalizing passes. At this point, critical edges are split by passLayoutBlocks.
func (b *builder) runPostBlockLayoutPasses() {
	if !b.doneBlockLayout {
		panic("runPostBlockLayoutPasses must be called after block layout pass is done")
	}
	// TODO: Do more.
	passTailDuplication(b)

	b.donePostBlockLayoutPasses = true
}

// runFinalizingPasses runs the finalizing passes. After this point, CFG should not be modified.
func (b *builder) runFinalizingPasses() {
	if !b.donePostBlockLayoutPasses {
		panic("runFinalizingPasses must be called after post block layout passes are done")
	}
	// Critical edges are split, so we fix the loop nesting forest.
	passBuildLoopNestingForest(b)
	passBuildDominatorTree(b)
	// Now that we know the final placement of the blocks, we can explicitly mark the fallthrough jumps.
	b.markFallthroughJumps()
}

// passDeadBlockEliminationOpt searches the unreachable blocks, and sets the basicBlock.invalid flag true if so.
func passDeadBlockEliminationOpt(b *builder) {
	entryBlk := b.entryBlk()
	b.clearBlkVisited()
	b.blkStack = append(b.blkStack, entryBlk)
	for len(b.blkStack) > 0 {
		reachableBlk := b.blkStack[len(b.blkStack)-1]
		b.blkStack = b.blkStack[:len(b.blkStack)-1]
		b.blkVisited[reachableBlk] = 0 // the value won't be used in this pass.

		if !reachableBlk.sealed && !reachableBlk.ReturnBlock() {
			panic(fmt.Sprintf("%s is not sealed", reachableBlk))
		}

		if wazevoapi.SSAValidationEnabled {
			reachableBlk.validate(b)
		}

		for _, succ := range reachableBlk.success {
			if _, ok := b.blkVisited[succ]; ok {
				continue
			}
			b.blkStack = append(b.blkStack, succ)
		}
	}

	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		if _, ok := b.blkVisited[blk]; !ok {
			blk.invalid = true
		}
	}
}

// passRedundantPhiEliminationOpt eliminates the redundant PHIs (in our terminology, parameters of a block).
func passRedundantPhiEliminationOpt(b *builder) {
	redundantParameterIndexes := b.ints[:0] // reuse the slice from previous iterations.

	_ = b.blockIteratorBegin() // skip entry block!
	// Below, we intentionally use the named iteration variable name, as this comes with inevitable nested for loops!
	for blk := b.blockIteratorNext(); blk != nil; blk = b.blockIteratorNext() {
		paramNum := len(blk.params)

		for paramIndex := 0; paramIndex < paramNum; paramIndex++ {
			phiValue := blk.params[paramIndex].value
			redundant := true

			nonSelfReferencingValue := ValueInvalid
			for predIndex := range blk.preds {
				pred := blk.preds[predIndex].branch.vs.View()[paramIndex]
				if pred == phiValue {
					// This is self-referencing: PHI from the same PHI.
					continue
				}

				if !nonSelfReferencingValue.Valid() {
					nonSelfReferencingValue = pred
					continue
				}

				if nonSelfReferencingValue != pred {
					redundant = false
					break
				}
			}

			if !nonSelfReferencingValue.Valid() {
				// This shouldn't happen, and must be a bug in builder.go.
				panic("BUG: params added but only self-referencing")
			}

			if redundant {
				b.redundantParameterIndexToValue[paramIndex] = nonSelfReferencingValue
				redundantParameterIndexes = append(redundantParameterIndexes, paramIndex)
			}
		}

		if len(b.redundantParameterIndexToValue) == 0 {
			continue
		}

		// Remove the redundant PHIs from the argument list of branching instructions.
		for predIndex := range blk.preds {
			var cur int
			predBlk := blk.preds[predIndex]
			branchInst := predBlk.branch
			view := branchInst.vs.View()
			for argIndex, value := range view {
				if _, ok := b.redundantParameterIndexToValue[argIndex]; !ok {
					view[cur] = value
					cur++
				}
			}
			branchInst.vs.Cut(cur)
		}

		// Still need to have the definition of the value of the PHI (previously as the parameter).
		for _, redundantParamIndex := range redundantParameterIndexes {
			phiValue := blk.params[redundantParamIndex].value
			onlyValue := b.redundantParameterIndexToValue[redundantParamIndex]
			// Create an alias in this block from the only phi argument to the phi value.
			b.alias(phiValue, onlyValue)
		}

		// Finally, Remove the param from the blk.
		var cur int
		for paramIndex := 0; paramIndex < paramNum; paramIndex++ {
			param := blk.params[paramIndex]
			if _, ok := b.redundantParameterIndexToValue[paramIndex]; !ok {
				blk.params[cur] = param
				cur++
			}
		}
		blk.params = blk.params[:cur]

		// Clears the map for the next iteration.
		for _, paramIndex := range redundantParameterIndexes {
			delete(b.redundantParameterIndexToValue, paramIndex)
		}
		redundantParameterIndexes = redundantParameterIndexes[:0]
	}

	// Reuse the slice for the future passes.
	b.ints = redundantParameterIndexes
}

// passDeadCodeEliminationOpt traverses all the instructions, and calculates the reference count of each Value, and
// eliminates all the unnecessary instructions whose ref count is zero.
// The results are stored at builder.valueRefCounts. This also assigns a InstructionGroupID to each Instruction
// during the process. This is the last SSA-level optimization pass and after this,
// the SSA function is ready to be used by backends.
//
// TODO: the algorithm here might not be efficient. Get back to this later.
func passDeadCodeEliminationOpt(b *builder) {
	nvid := int(b.nextValueID)
	if nvid >= len(b.valueRefCounts) {
		b.valueRefCounts = append(b.valueRefCounts, make([]int, b.nextValueID)...)
	}
	if nvid >= len(b.valueIDToInstruction) {
		b.valueIDToInstruction = append(b.valueIDToInstruction, make([]*Instruction, b.nextValueID)...)
	}

	// First, we gather all the instructions with side effects.
	liveInstructions := b.instStack[:0]
	// During the process, we will assign InstructionGroupID to each instruction, which is not
	// relevant to dead code elimination, but we need in the backend.
	var gid InstructionGroupID
	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for cur := blk.rootInstr; cur != nil; cur = cur.next {
			cur.gid = gid
			switch cur.sideEffect() {
			case sideEffectTraps:
				// The trappable should always be alive.
				liveInstructions = append(liveInstructions, cur)
			case sideEffectStrict:
				liveInstructions = append(liveInstructions, cur)
				// The strict side effect should create different instruction groups.
				gid++
			}

			r1, rs := cur.Returns()
			if r1.Valid() {
				b.valueIDToInstruction[r1.ID()] = cur
			}
			for _, r := range rs {
				b.valueIDToInstruction[r.ID()] = cur
			}
		}
	}

	// Find all the instructions referenced by live instructions transitively.
	for len(liveInstructions) > 0 {
		tail := len(liveInstructions) - 1
		live := liveInstructions[tail]
		liveInstructions = liveInstructions[:tail]
		if live.live {
			// If it's already marked alive, this is referenced multiple times,
			// so we can skip it.
			continue
		}
		live.live = true

		// Before we walk, we need to resolve the alias first.
		b.resolveArgumentAlias(live)

		v1, v2, v3, vs := live.Args()
		if v1.Valid() {
			producingInst := b.valueIDToInstruction[v1.ID()]
			if producingInst != nil {
				liveInstructions = append(liveInstructions, producingInst)
			}
		}

		if v2.Valid() {
			producingInst := b.valueIDToInstruction[v2.ID()]
			if producingInst != nil {
				liveInstructions = append(liveInstructions, producingInst)
			}
		}

		if v3.Valid() {
			producingInst := b.valueIDToInstruction[v3.ID()]
			if producingInst != nil {
				liveInstructions = append(liveInstructions, producingInst)
			}
		}

		for _, v := range vs {
			producingInst := b.valueIDToInstruction[v.ID()]
			if producingInst != nil {
				liveInstructions = append(liveInstructions, producingInst)
			}
		}
	}

	// Now that all the live instructions are flagged as live=true, we eliminate all dead instructions.
	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for cur := blk.rootInstr; cur != nil; cur = cur.next {
			if !cur.live {
				// Remove the instruction from the list.
				if prev := cur.prev; prev != nil {
					prev.next = cur.next
				} else {
					blk.rootInstr = cur.next
				}
				if next := cur.next; next != nil {
					next.prev = cur.prev
				}
				continue
			}

			// If the value alive, we can be sure that arguments are used definitely.
			// Hence, we can increment the value reference counts.
			v1, v2, v3, vs := cur.Args()
			if v1.Valid() {
				b.incRefCount(v1.ID(), cur)
			}
			if v2.Valid() {
				b.incRefCount(v2.ID(), cur)
			}
			if v3.Valid() {
				b.incRefCount(v3.ID(), cur)
			}
			for _, v := range vs {
				b.incRefCount(v.ID(), cur)
			}
		}
	}

	b.instStack = liveInstructions // we reuse the stack for the next iteration.
}

func (b *builder) incRefCount(id ValueID, from *Instruction) {
	if wazevoapi.SSALoggingEnabled {
		fmt.Printf("v%d referenced from %v\n", id, from.Format(b))
	}
	b.valueRefCounts[id]++
}

// clearBlkVisited clears the b.blkVisited map so that we can reuse it for multiple places.
func (b *builder) clearBlkVisited() {
	b.blkStack2 = b.blkStack2[:0]
	for key := range b.blkVisited {
		b.blkStack2 = append(b.blkStack2, key)
	}
	for _, blk := range b.blkStack2 {
		delete(b.blkVisited, blk)
	}
	b.blkStack2 = b.blkStack2[:0]
}

// passNopInstElimination eliminates the instructions which is essentially a no-op.
func passNopInstElimination(b *builder) {
	if int(b.nextValueID) >= len(b.valueIDToInstruction) {
		b.valueIDToInstruction = append(b.valueIDToInstruction, make([]*Instruction, b.nextValueID)...)
	}

	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for cur := blk.rootInstr; cur != nil; cur = cur.next {
			r1, rs := cur.Returns()
			if r1.Valid() {
				b.valueIDToInstruction[r1.ID()] = cur
			}
			for _, r := range rs {
				b.valueIDToInstruction[r.ID()] = cur
			}
		}
	}

	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for cur := blk.rootInstr; cur != nil; cur = cur.next {
			switch cur.Opcode() {
			// TODO: add more logics here.
			case OpcodeIshl, OpcodeSshr, OpcodeUshr:
				x, amount := cur.Arg2()
				definingInst := b.valueIDToInstruction[amount.ID()]
				if definingInst == nil {
					// If there's no defining instruction, that means the amount is coming from the parameter.
					continue
				}
				if definingInst.Constant() {
					v := definingInst.ConstantVal()

					if x.Type().Bits() == 64 {
						v = v % 64
					} else {
						v = v % 32
					}
					if v == 0 {
						b.alias(cur.Return(), x)
					}
				}
			}
		}
	}
}

// passSortSuccessors sorts the successors of each block in the natural program order.
func passSortSuccessors(b *builder) {
	for i := 0; i < b.basicBlocksPool.Allocated(); i++ {
		blk := b.basicBlocksPool.View(i)
		sortBlocks(blk.success)
	}
}

// passTailDuplication duplicates the block that has many predecessors while using many values if the block is simple enough.
// This is similar to TailDuplication in LLVM:
// https://opensource.apple.com/source/clang/clang-137/src/lib/Transforms/Scalar/TailDuplication.cpp.auto.html
func passTailDuplication(b *builder) {
	for i := 0; i < b.basicBlocksPool.Allocated(); i++ {
		blk := b.basicBlocksPool.View(i)
		if isTailDuplicationTarget(blk) {
			doTailDuplication(b, blk)
		}
	}
}

func isTailDuplicationTarget(blk *basicBlock) (ok bool) {
	if blk.ReturnBlock() || len(blk.preds) < 5 || len(blk.params) == 0 {
		return
	}

	var usedValueCount int
	for instrCount, cur := 0, blk.rootInstr; cur != nil; cur = cur.next {
		if len(cur.rValues.View()) > 0 {
			// If an instruction has many values, we don't duplicate the block to avoid the complexity.
			return
		}
		if instrCount > 10 {
			return
		}
		if cur.v.Valid() {
			usedValueCount++
		}
		if cur.v2.Valid() {
			usedValueCount++
		}

		if cur.v3.Valid() {
			usedValueCount++
		}
		view := cur.vs.View()
		for range view {
			usedValueCount++
		}
		instrCount++
	}

	if usedValueCount < 10 {
		return
	}

	// TODO: allow Return target.
	if tail := blk.Tail(); tail.opcode != OpcodeJump || tail.blk.ReturnBlock() {
		return
	}

	// TODO: it should be possible to duplicate the block if it contains conditional jump if the destinations are dominated by it.
	// 	We should implement this later but not sure if it's worth it.
	if len(blk.success) > 1 {
		return
	}

	// At this point,
	// * the jump is unconditional (we only have one successor).
	// * the destination has at least 5 predecessors.
	// * the destination has at most 10 instructions.
	// * the destination has at most 10 used values.
	//
	// therefore, before making this jump, it is highly likely that register state reconciliation is necessary due to
	// the high number of predecessors and the high number of used values.
	// So, we duplicate the destination block at the end of the predecessor block so that we won't need to reconcile the register state.
	ok = true
	return
}

func doTailDuplication(b *builder, eliminationTarget *basicBlock) {
	// Sanity check to ensure all origin jumps are unconditional.
	for i := range eliminationTarget.preds {
		if eliminationTarget.preds[i].branch.opcode != OpcodeJump {
			panic("BUG: eliminationTarget has at least one param, and therefore all predecessors" +
				" should be unconditional jumps since this is after critical edge splitting")
		}
	}
	if len(eliminationTarget.success) != 1 {
		panic("BUG: eliminationTarget has more than one successor")
	}
	dst := eliminationTarget.success[0]

	for i := range dst.preds {
		if dst.preds[i].blk == eliminationTarget {
			// Remove the edge from the dst.preds.
			dst.preds = append(dst.preds[:i], dst.preds[i+1:]...)
			break
		}
	}

	// First we need to expand the block params of the destination block by the values
	// defined by the eliminationTarget block.
	dstIsDominatedByEliminationTarget := b.isDominatedBy(dst, eliminationTarget)
	if dstIsDominatedByEliminationTarget {
		// If the dst is dominated by the eliminationTarget, the locally defined values in eliminationTarget
		// might be used in the dst and the following blocks. Otherwise, the value must be already passed as a parameter.
		//
		// So, we gather the locally defined values in the eliminationTarget block.
		for cur := eliminationTarget.rootInstr; cur != nil; cur = cur.next {
			v := cur.rValue
			if v.Valid() {
				dst.params = append(dst.params, blockParam{value: v, typ: v.Type()})
			}
		}
	}

	// Then merge the instructions of the eliminationTarget block into the predecessors.
	var valueMapping map[Value]Value
	additionalParams := make([]Value, 0, 10) // I believe this doesn't cause an allocation since we won't have more than 10 additional params.
	for i := range eliminationTarget.preds {
		valueMapping = wazevoapi.ResetMap(valueMapping)
		additionalParams = additionalParams[:0]

		pred := &eliminationTarget.preds[i]
		predJmp, predBlk := pred.branch, pred.blk

		// First, we need to create mapping from the arguments to the elimination target block.
		for i, arg := range predJmp.vs.View() {
			phi := eliminationTarget.params[i]
			valueMapping[phi.value] = arg
		}

		gid := predJmp.gid

		// Then, walk through the instructions and duplicate them.
		cur := predJmp.prev
		for dupCur := eliminationTarget.rootInstr; dupCur != nil; dupCur = dupCur.next {
			copied := b.AllocateInstruction()
			id := copied.id
			*copied = *dupCur
			copied.id = id
			copied.gid = gid
			if copied.sideEffect() == sideEffectStrict {
				gid++
			}

			originalResult := dupCur.Return()
			if originalResult.Valid() {
				newResult := b.allocateValue(originalResult.Type())
				valueMapping[originalResult] = newResult
				copied.rValue = newResult
				additionalParams = append(additionalParams, newResult)
			}

			// Resolve the mapped values.
			if copied.v.Valid() {
				if mapped, ok := valueMapping[copied.v]; ok {
					copied.v = mapped
				}
			}

			if copied.v2.Valid() {
				if mapped, ok := valueMapping[copied.v2]; ok {
					copied.v2 = mapped
				}
			}

			if copied.v3.Valid() {
				if mapped, ok := valueMapping[copied.v3]; ok {
					copied.v3 = mapped
				}
			}

			newVs := b.AllocateVarLengthValues(copied.vs.View()...)
			newVsView := newVs.View()
			for i, v := range newVsView {
				if mapped, ok := valueMapping[v]; ok {
					newVsView[i] = mapped
				}
			}

			copied.vs = newVs

			// Insert the new instruction to insertionCur.
			if cur == nil {
				predBlk.rootInstr = copied
			} else {
				cur.next = copied
				copied.prev = cur
			}
			cur = copied

			// Finally, append the new instruction to the predJmp.
			if copied.opcode == OpcodeJump {
				// If the dest is dst, we need to append the params.
				if dstIsDominatedByEliminationTarget {
					for _, v := range additionalParams {
						newVs = newVs.Append(&b.varLengthPool, v)
					}
				}
				dst.preds = append(dst.preds, basicBlockPredecessorInfo{branch: predJmp, blk: predBlk})
			}
		}
		predBlk.currentInstr = cur
	}

	eliminationTarget.invalid = true
}
