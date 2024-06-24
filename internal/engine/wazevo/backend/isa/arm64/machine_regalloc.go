package arm64

// This file implements the interfaces required for register allocations. See backend.RegAllocFunctionMachine.

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type regAllocFn struct {
	ssaB                   ssa.Builder
	m                      *machine
	loopNestingForestRoots []ssa.BasicBlock
	blockIter              int
}

// PostOrderBlockIteratorBegin implements regalloc.Function.
func (f *regAllocFn) PostOrderBlockIteratorBegin() regalloc.Block {
	f.blockIter = len(f.m.orderedLabelPos) - 1
	return f.PostOrderBlockIteratorNext()
}

// PostOrderBlockIteratorNext implements regalloc.Function.
func (f *regAllocFn) PostOrderBlockIteratorNext() regalloc.Block {
	if f.blockIter < 0 {
		return nil
	}
	b := f.m.orderedLabelPos[f.blockIter]
	f.blockIter--
	return b
}

// ReversePostOrderBlockIteratorBegin implements regalloc.Function.
func (f *regAllocFn) ReversePostOrderBlockIteratorBegin() regalloc.Block {
	f.blockIter = 0
	return f.ReversePostOrderBlockIteratorNext()
}

// ReversePostOrderBlockIteratorNext implements regalloc.Function.
func (f *regAllocFn) ReversePostOrderBlockIteratorNext() regalloc.Block {
	if f.blockIter >= len(f.m.orderedLabelPos) {
		return nil
	}
	b := f.m.orderedLabelPos[f.blockIter]
	f.blockIter++
	return b
}

// ClobberedRegisters implements regalloc.Function.
func (f *regAllocFn) ClobberedRegisters(regs []regalloc.VReg) {
	f.m.clobberedRegs = append(f.m.clobberedRegs[:0], regs...)
}

// LoopNestingForestRoots implements regalloc.Function.
func (f *regAllocFn) LoopNestingForestRoots() int {
	f.loopNestingForestRoots = f.ssaB.LoopNestingForestRoots()
	return len(f.loopNestingForestRoots)
}

// LoopNestingForestRoot implements regalloc.Function.
func (f *regAllocFn) LoopNestingForestRoot(i int) regalloc.Block {
	root := f.loopNestingForestRoots[i]
	pos := f.m.getOrAllocateSSABlockLabelPosition(root)
	return pos
}

// LowestCommonAncestor implements regalloc.Function.
func (f *regAllocFn) LowestCommonAncestor(blk1, blk2 regalloc.Block) regalloc.Block {
	sb := f.ssaB.LowestCommonAncestor(blk1.(*labelPosition).sb, blk2.(*labelPosition).sb)
	pos := f.m.getOrAllocateSSABlockLabelPosition(sb)
	return pos
}

// Idom implements regalloc.Function.
func (f *regAllocFn) Idom(blk regalloc.Block) regalloc.Block {
	sb := f.ssaB.Idom(blk.(*labelPosition).sb)
	pos := f.m.getOrAllocateSSABlockLabelPosition(sb)
	return pos
}

// SwapBefore implements regalloc.Function.
func (f *regAllocFn) SwapBefore(x1, x2, tmp regalloc.VReg, instr regalloc.Instr) {
	f.m.swap(instr.Prev().(*instruction), x1, x2, tmp)
}

// StoreRegisterBefore implements regalloc.Function.
func (f *regAllocFn) StoreRegisterBefore(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertStoreRegisterAt(v, instr.(*instruction), false)
}

// StoreRegisterAfter implements regalloc.Function.
func (f *regAllocFn) StoreRegisterAfter(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertStoreRegisterAt(v, instr.(*instruction), true)
}

// ReloadRegisterBefore implements regalloc.Function.
func (f *regAllocFn) ReloadRegisterBefore(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertReloadRegisterAt(v, instr.(*instruction), false)
}

// ReloadRegisterAfter implements regalloc.Function.
func (f *regAllocFn) ReloadRegisterAfter(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertReloadRegisterAt(v, instr.(*instruction), true)
}

// InsertMoveBefore implements regalloc.Function.
func (f *regAllocFn) InsertMoveBefore(dst, src regalloc.VReg, instr regalloc.Instr) {
	f.m.insertMoveBefore(dst, src, instr.(*instruction))
}

func (pos *labelPosition) ID() int32 {
	return int32(pos.sb.ID())
}

// LoopNestingForestChild implements regalloc.Block.
func (pos *labelPosition) LoopNestingForestChild(i int) regalloc.Block {
	childSB := pos.sb.LoopNestingForestChildren()[i]
	return pos.m.getOrAllocateSSABlockLabelPosition(childSB)
}

// Succ implements regalloc.Block.
func (pos *labelPosition) Succ(i int) regalloc.Block {
	succSB := pos.sb.Succ(i)
	if succSB.ReturnBlock() {
		return nil
	}
	return pos.m.getOrAllocateSSABlockLabelPosition(succSB)
}

// Pred implements regalloc.Block.
func (pos *labelPosition) Pred(i int) regalloc.Block {
	predSB := pos.sb.Pred(i)
	return pos.m.getOrAllocateSSABlockLabelPosition(predSB)
}

// BlockParams implements regalloc.Block.
func (pos *labelPosition) BlockParams(regs *[]regalloc.VReg) []regalloc.VReg {
	c := pos.m.compiler
	*regs = (*regs)[:0]
	for i := 0; i < pos.sb.Params(); i++ {
		v := c.VRegOf(pos.sb.Param(i))
		*regs = append(*regs, v)
	}
	return *regs
}

// InstrIteratorBegin implements regalloc.Block.
func (pos *labelPosition) InstrIteratorBegin() regalloc.Instr {
	ret := pos.begin
	pos.cur = ret
	return ret
}

// InstrIteratorNext implements regalloc.Block.
func (pos *labelPosition) InstrIteratorNext() regalloc.Instr {
	for {
		if pos.cur == pos.end {
			return nil
		}
		instr := pos.cur.next
		pos.cur = instr
		if instr == nil {
			return nil
		} else if instr.AddedBeforeRegAlloc() {
			// Only concerned about the instruction added before regalloc.
			return instr
		}
	}
}

// InstrRevIteratorBegin implements regalloc.Block.
func (pos *labelPosition) InstrRevIteratorBegin() regalloc.Instr {
	pos.cur = pos.end
	return pos.cur
}

// InstrRevIteratorNext implements regalloc.Block.
func (pos *labelPosition) InstrRevIteratorNext() regalloc.Instr {
	for {
		if pos.cur == pos.begin {
			return nil
		}
		instr := pos.cur.prev
		pos.cur = instr
		if instr == nil {
			return nil
		} else if instr.AddedBeforeRegAlloc() {
			// Only concerned about the instruction added before regalloc.
			return instr
		}
	}
}

// FirstInstr implements regalloc.Block.
func (pos *labelPosition) FirstInstr() regalloc.Instr { return pos.begin }

// LastInstrForInsertion implements regalloc.Block.
func (pos *labelPosition) LastInstrForInsertion() regalloc.Instr {
	return lastInstrForInsertion(pos.begin, pos.end)
}

// Preds implements regalloc.Block.
func (pos *labelPosition) Preds() int { return pos.sb.Preds() }

// Entry implements regalloc.Block.
func (pos *labelPosition) Entry() bool { return pos.sb.EntryBlock() }

// Succs implements regalloc.Block.
func (pos *labelPosition) Succs() int { return pos.sb.Succs() }

// LoopHeader implements regalloc.Block.
func (pos *labelPosition) LoopHeader() bool { return pos.sb.LoopHeader() }

// LoopNestingForestChildren implements regalloc.Block.
func (pos *labelPosition) LoopNestingForestChildren() int {
	return len(pos.sb.LoopNestingForestChildren())
}

func (m *machine) swap(cur *instruction, x1, x2, tmp regalloc.VReg) {
	prevNext := cur.next
	var mov1, mov2, mov3 *instruction
	if x1.RegType() == regalloc.RegTypeInt {
		if !tmp.Valid() {
			tmp = tmpRegVReg
		}
		mov1 = m.allocateInstr().asMove64(tmp, x1)
		mov2 = m.allocateInstr().asMove64(x1, x2)
		mov3 = m.allocateInstr().asMove64(x2, tmp)
		cur = linkInstr(cur, mov1)
		cur = linkInstr(cur, mov2)
		cur = linkInstr(cur, mov3)
		linkInstr(cur, prevNext)
	} else {
		if !tmp.Valid() {
			r2 := x2.RealReg()
			// Temporarily spill x1 to stack.
			cur = m.insertStoreRegisterAt(x1, cur, true).prev
			// Then move x2 to x1.
			cur = linkInstr(cur, m.allocateInstr().asFpuMov128(x1, x2))
			linkInstr(cur, prevNext)
			// Then reload the original value on x1 from stack to r2.
			m.insertReloadRegisterAt(x1.SetRealReg(r2), cur, true)
		} else {
			mov1 = m.allocateInstr().asFpuMov128(tmp, x1)
			mov2 = m.allocateInstr().asFpuMov128(x1, x2)
			mov3 = m.allocateInstr().asFpuMov128(x2, tmp)
			cur = linkInstr(cur, mov1)
			cur = linkInstr(cur, mov2)
			cur = linkInstr(cur, mov3)
			linkInstr(cur, prevNext)
		}
	}
}

func (m *machine) insertMoveBefore(dst, src regalloc.VReg, instr *instruction) {
	typ := src.RegType()
	if typ != dst.RegType() {
		panic("BUG: src and dst must have the same type")
	}

	mov := m.allocateInstr()
	if typ == regalloc.RegTypeInt {
		mov.asMove64(dst, src)
	} else {
		mov.asFpuMov128(dst, src)
	}

	cur := instr.prev
	prevNext := cur.next
	cur = linkInstr(cur, mov)
	linkInstr(cur, prevNext)
}

func (m *machine) insertStoreRegisterAt(v regalloc.VReg, instr *instruction, after bool) *instruction {
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.compiler.TypeOf(v)

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	offsetFromSP := m.getVRegSpillSlotOffsetFromSP(v.ID(), typ.Size())
	var amode *addressMode
	cur, amode = m.resolveAddressModeForOffsetAndInsert(cur, offsetFromSP, typ.Bits(), spVReg, true)
	store := m.allocateInstr()
	store.asStore(operandNR(v), amode, typ.Bits())

	cur = linkInstr(cur, store)
	return linkInstr(cur, prevNext)
}

func (m *machine) insertReloadRegisterAt(v regalloc.VReg, instr *instruction, after bool) *instruction {
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.compiler.TypeOf(v)

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	offsetFromSP := m.getVRegSpillSlotOffsetFromSP(v.ID(), typ.Size())
	var amode *addressMode
	cur, amode = m.resolveAddressModeForOffsetAndInsert(cur, offsetFromSP, typ.Bits(), spVReg, true)
	load := m.allocateInstr()
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		load.asULoad(v, amode, typ.Bits())
	case ssa.TypeF32, ssa.TypeF64:
		load.asFpuLoad(v, amode, typ.Bits())
	case ssa.TypeV128:
		load.asFpuLoad(v, amode, 128)
	default:
		panic("TODO")
	}

	cur = linkInstr(cur, load)
	return linkInstr(cur, prevNext)
}

func lastInstrForInsertion(begin, end *instruction) *instruction {
	cur := end
	for cur.kind == nop0 {
		cur = cur.prev
		if cur == begin {
			return end
		}
	}
	switch cur.kind {
	case br:
		return cur
	default:
		return end
	}
}
