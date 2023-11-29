package ssa

import (
	"fmt"
	"math"
	"sort"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// RunPasses implements Builder.RunPasses.
//
// The order here matters; some pass depends on the previous ones.
//
// Note that passes suffixed with "Opt" are the optimization passes, meaning that they edit the instructions and blocks
// while the other passes are not, like passEstimateBranchProbabilities does not edit them, but only calculates the additional information.
func (b *builder) RunPasses() {
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

	passConstFoldingOpt(b)

	// passDeadCodeEliminationOpt could be more accurate if we do this after other optimizations.
	passDeadCodeEliminationOpt(b)
	b.donePasses = true
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
				pred := blk.preds[predIndex].branch.vs[paramIndex]
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
			for argIndex, value := range branchInst.vs {
				if _, ok := b.redundantParameterIndexToValue[argIndex]; !ok {
					branchInst.vs[cur] = value
					cur++
				}
			}
			branchInst.vs = branchInst.vs[:cur]
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
	ensureValueIdToInstructionInit(b)

	for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
		for cur := blk.rootInstr; cur != nil; cur = cur.next {
			op := cur.Opcode()
			switch op {
			// TODO: add more logics here.
			// Amount := (Const $someValue)
			// (Shift X, Amount) where Amount == x.Type.Bits() => X
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
			// Z := Const 0
			// (Iadd X, Z) => X
			// (Iadd Z, Y) => Y
			case OpcodeIadd:
				x, y := cur.Arg2()
				definingInst := b.valueIDToInstruction[y.ID()]
				if definingInst == nil {
					if definingInst = b.valueIDToInstruction[x.ID()]; definingInst == nil {
						continue
					} else {
						x = y
					}
				}
				if definingInst.Constant() && definingInst.ConstantVal() == 0 {
					b.alias(cur.Return(), x)
				}
			}
		}
	}
}

func ensureValueIdToInstructionInit(b *builder) {
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
}

// passConstFoldingOpt scans all instructions for arithmetic operations over constants,
// and replaces them with a const of their result.
func passConstFoldingOpt(b *builder) {
	ensureValueIdToInstructionInit(b)

	isFixedPoint := false
	for !isFixedPoint {
		isFixedPoint = true
		for blk := b.blockIteratorBegin(); blk != nil; blk = b.blockIteratorNext() {
			for cur := blk.rootInstr; cur != nil; cur = cur.next {
				op := cur.Opcode()
				switch op {
				// X := Const xc
				// Y := Const yc
				// - (Iadd X, Y) => Const (xc + yc)
				case OpcodeIadd, OpcodeIsub, OpcodeImul:
					x, y := cur.Arg2()
					xDef := b.valueIDToInstruction[x.ID()]
					yDef := b.valueIDToInstruction[y.ID()]
					if xDef == nil || yDef == nil {
						// If we are adding some parameter, ignore.
						continue
					}
					if xDef.Constant() && yDef.Constant() {
						isFixedPoint = false
						// Mutate the instruction to an Iconst.
						cur.opcode = OpcodeIconst
						// Clear the references to operands.
						cur.v, cur.v2 = ValueInvalid, ValueInvalid
						// We assume all the types are consistent.
						xc, yc := xDef.ConstantVal(), yDef.ConstantVal()
						switch op {
						case OpcodeIadd:
							cur.u1 = xc + yc
						case OpcodeIsub:
							cur.u1 = xc - yc
						case OpcodeImul:
							cur.u1 = xc * yc
						}
					}
				case OpcodeFadd, OpcodeFsub, OpcodeFmul:
					x, y := cur.Arg2()
					xDef := b.valueIDToInstruction[x.ID()]
					yDef := b.valueIDToInstruction[y.ID()]
					if xDef == nil || yDef == nil {
						// If we are adding together some parameter, ignore.
						continue
					}
					if xDef.Constant() && yDef.Constant() {
						isFixedPoint = false
						// Mutate the instruction to an Iconst.
						// Clear the references to operands.
						cur.v, cur.v2 = ValueInvalid, ValueInvalid
						// We assume all the types are consistent.
						if x.Type().Bits() == 64 {
							cur.opcode = OpcodeF64const
							yc := math.Float64frombits(yDef.ConstantVal())
							xc := math.Float64frombits(xDef.ConstantVal())
							switch op {
							case OpcodeFadd:
								cur.u1 = math.Float64bits(xc + yc)
							case OpcodeFsub:
								cur.u1 = math.Float64bits(xc - yc)
							case OpcodeFmul:
								cur.u1 = math.Float64bits(xc * yc)
							}
						} else {
							cur.opcode = OpcodeF32const
							yc := math.Float32frombits(uint32(yDef.ConstantVal()))
							xc := math.Float32frombits(uint32(xDef.ConstantVal()))
							switch op {
							case OpcodeFadd:
								cur.u1 = uint64(math.Float32bits(xc + yc))
							case OpcodeFsub:
								cur.u1 = uint64(math.Float32bits(xc - yc))
							case OpcodeFmul:
								cur.u1 = uint64(math.Float32bits(xc * yc))
							}
						}
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
		sort.SliceStable(blk.success, func(i, j int) bool {
			iBlk, jBlk := blk.success[i], blk.success[j]
			if jBlk.ReturnBlock() {
				return true
			}
			if iBlk.ReturnBlock() {
				return false
			}
			iRoot, jRoot := iBlk.rootInstr, jBlk.rootInstr
			if iRoot == nil || jRoot == nil { // For testing.
				return true
			}
			return iBlk.rootInstr.id < jBlk.rootInstr.id
		})
	}
}
