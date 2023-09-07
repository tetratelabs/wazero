package arm64

// This file implements the interfaces required for register allocations. See regalloc/api.go.

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type (
	// regAllocFunctionImpl implements regalloc.Function.
	regAllocFunctionImpl struct {
		m *machine
		// iter is the iterator for reversePostOrderBlocks.
		iter                   int
		reversePostOrderBlocks []regAllocBlockImpl
		// labelToRegAllocBlockIndex maps label to the index of reversePostOrderBlocks.
		labelToRegAllocBlockIndex map[label]int
		// vs is used for regalloc.Instr Defs() and Uses() methods, defined here for reuse.
		vs []regalloc.VReg
	}

	// regAllocBlockImpl implements regalloc.Block.
	regAllocBlockImpl struct {
		// f is the function this instruction belongs to. Used to reuse the regAllocFunctionImpl.predsSlice slice for Defs() and Uses().
		f   *regAllocFunctionImpl
		sb  ssa.BasicBlock
		l   label
		pos *labelPosition
		// instrImpl is re-used for all instructions in this block.
		instrImpl regAllocInstrImpl
	}

	// regAllocInstrImpl implements regalloc.Instr.
	regAllocInstrImpl struct {
		// f is the function this instruction belongs to. Used to reuse the regAllocFunctionImpl.vs slice for Defs() and Uses().
		f *regAllocFunctionImpl
		i *instruction
	}
)

func (f *regAllocFunctionImpl) addBlock(sb ssa.BasicBlock, l label, pos *labelPosition) {
	i := len(f.reversePostOrderBlocks)
	f.reversePostOrderBlocks = append(f.reversePostOrderBlocks, regAllocBlockImpl{
		f:         f,
		sb:        sb,
		l:         l,
		pos:       pos,
		instrImpl: regAllocInstrImpl{f: f},
	})
	f.labelToRegAllocBlockIndex[l] = i
}

func (f *regAllocFunctionImpl) reset() {
	f.reversePostOrderBlocks = f.reversePostOrderBlocks[:0]
	f.vs = f.vs[:0]
	f.iter = 0
}

var (
	_ regalloc.Function = (*regAllocFunctionImpl)(nil)
	_ regalloc.Block    = (*regAllocBlockImpl)(nil)
	_ regalloc.Instr    = (*regAllocInstrImpl)(nil)
)

// PostOrderBlockIteratorBegin implements regalloc.Function PostOrderBlockIteratorBegin.
func (f *regAllocFunctionImpl) PostOrderBlockIteratorBegin() regalloc.Block {
	f.iter = len(f.reversePostOrderBlocks) - 1
	return f.PostOrderBlockIteratorNext()
}

// PostOrderBlockIteratorNext implements regalloc.Function PostOrderBlockIteratorNext.
func (f *regAllocFunctionImpl) PostOrderBlockIteratorNext() regalloc.Block {
	if f.iter < 0 {
		return nil
	}
	b := &f.reversePostOrderBlocks[f.iter]
	f.iter--
	return b
}

// ReversePostOrderBlockIteratorBegin implements regalloc.Function ReversePostOrderBlockIteratorBegin.
func (f *regAllocFunctionImpl) ReversePostOrderBlockIteratorBegin() regalloc.Block {
	f.iter = 0
	return f.ReversePostOrderBlockIteratorNext()
}

// ReversePostOrderBlockIteratorNext implements regalloc.Function ReversePostOrderBlockIteratorNext.
func (f *regAllocFunctionImpl) ReversePostOrderBlockIteratorNext() regalloc.Block {
	if f.iter >= len(f.reversePostOrderBlocks) {
		return nil
	}
	b := &f.reversePostOrderBlocks[f.iter]
	f.iter++
	return b
}

// ClobberedRegisters implements regalloc.Function ClobberedRegisters.
func (f *regAllocFunctionImpl) ClobberedRegisters(regs []regalloc.VReg) {
	m := f.m
	m.clobberedRegs = append(m.clobberedRegs[:0], regs...)
}

// StoreRegisterBefore implements regalloc.Function StoreRegisterBefore.
func (f *regAllocFunctionImpl) StoreRegisterBefore(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertStoreRegisterAt(v, instr.(*regAllocInstrImpl).i, false)
}

// StoreRegisterAfter implements regalloc.Function StoreRegisterAfter.
func (f *regAllocFunctionImpl) StoreRegisterAfter(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.insertStoreRegisterAt(v, instr.(*regAllocInstrImpl).i, true)
}

// ReloadRegisterBefore implements regalloc.Function ReloadRegisterBefore.
func (f *regAllocFunctionImpl) ReloadRegisterBefore(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.reloadRegister(v, instr.(*regAllocInstrImpl).i, false)
}

// ReloadRegisterAfter implements regalloc.Function ReloadRegisterAfter.
func (f *regAllocFunctionImpl) ReloadRegisterAfter(v regalloc.VReg, instr regalloc.Instr) {
	m := f.m
	m.reloadRegister(v, instr.(*regAllocInstrImpl).i, true)
}

// Done implements regalloc.Function Done.
func (f *regAllocFunctionImpl) Done() {
	m := f.m
	// Now that we know the final spill slot size, we must align spillSlotSize to 16 bytes.
	m.spillSlotSize = (m.spillSlotSize + 15) &^ 15
}

// ID implements regalloc.Block ID.
func (r *regAllocBlockImpl) ID() int {
	return int(r.sb.ID())
}

// Preds implements regalloc.Block Preds.
func (r *regAllocBlockImpl) Preds() int {
	return r.sb.Preds()
}

// Pred implements regalloc.Block Pred.
func (r *regAllocBlockImpl) Pred(i int) regalloc.Block {
	sb := r.sb
	pred := sb.Pred(i)
	l := r.f.m.ssaBlockIDToLabels[pred.ID()]
	index := r.f.labelToRegAllocBlockIndex[l]
	return &r.f.reversePostOrderBlocks[index]
}

// InstrIteratorBegin implements regalloc.Block InstrIteratorBegin.
func (r *regAllocBlockImpl) InstrIteratorBegin() regalloc.Instr {
	r.instrImpl.i = r.pos.begin
	return &r.instrImpl
}

// InstrIteratorNext implements regalloc.Block InstrIteratorNext.
func (r *regAllocBlockImpl) InstrIteratorNext() regalloc.Instr {
	for {
		instr := r.instrIteratorNext()
		if instr == nil {
			return nil
		} else if !instr.i.addedAfterLowering {
			// Skips the instruction added after lowering.
			return instr
		}
	}
}

// BlockParams implements regalloc.Block BlockParams.
func (r *regAllocBlockImpl) BlockParams() []regalloc.VReg {
	c := r.f.m.compiler
	regs := r.f.vs[:0]
	for i := 0; i < r.sb.Params(); i++ {
		v := c.VRegOf(r.sb.Param(i))
		regs = append(regs, v)
	}
	return regs
}

func (r *regAllocBlockImpl) instrIteratorNext() *regAllocInstrImpl {
	cur := r.instrImpl.i
	if r.pos.end == cur {
		return nil
	}
	r.instrImpl.i = cur.next
	return &r.instrImpl
}

// Entry implements regalloc.Block Entry.
func (r *regAllocBlockImpl) Entry() bool { return r.sb.EntryBlock() }

// Format implements regalloc.Instr String.
func (r *regAllocInstrImpl) String() string {
	return r.i.String()
}

// Defs implements regalloc.Instr Defs.
func (r *regAllocInstrImpl) Defs() []regalloc.VReg {
	regs := r.f.vs[:0]
	regs = r.i.defs(regs)
	r.f.vs = regs
	return regs
}

// Uses implements regalloc.Instr Uses.
func (r *regAllocInstrImpl) Uses() []regalloc.VReg {
	regs := r.f.vs[:0]
	regs = r.i.uses(regs)
	r.f.vs = regs
	return regs
}

// IsCopy implements regalloc.Instr IsCopy.
func (r *regAllocInstrImpl) IsCopy() bool {
	return r.i.isCopy()
}

// RegisterInfo implements backend.Machine.
func (m *machine) RegisterInfo(debug bool) *regalloc.RegisterInfo {
	if debug {
		regInfoDebug := &regalloc.RegisterInfo{}
		regInfoDebug.CalleeSavedRegisters = regInfo.CalleeSavedRegisters
		regInfoDebug.CallerSavedRegisters = regInfo.CallerSavedRegisters
		regInfoDebug.RealRegToVReg = regInfo.RealRegToVReg
		regInfoDebug.RealRegName = regInfo.RealRegName
		regInfoDebug.AllocatableRegisters[regalloc.RegTypeFloat] = []regalloc.RealReg{
			v18,                            // One callee saved.
			v7, v6, v5, v4, v3, v2, v1, v0, // Allocatable sets == Argument registers.
		}
		regInfoDebug.AllocatableRegisters[regalloc.RegTypeInt] = []regalloc.RealReg{
			x29, x30, // Caller saved, and special ones. But they should be able to get allocated.
			x19,                            // One callee saved.
			x7, x6, x5, x4, x3, x2, x1, x0, // Argument registers (all caller saved).
		}
		return regInfoDebug
	}
	return regInfo
}

// Function implements backend.Machine Function.
func (m *machine) Function() regalloc.Function {
	return &m.regAllocFn
}

// IsCall implements regalloc.Instr IsCall.
func (r *regAllocInstrImpl) IsCall() bool {
	return r.i.kind == call
}

// IsIndirectCall implements regalloc.Instr IsIndirectCall.
func (r *regAllocInstrImpl) IsIndirectCall() bool {
	return r.i.kind == callInd
}

// IsReturn implements regalloc.Instr IsReturn.
func (r *regAllocInstrImpl) IsReturn() bool {
	return r.i.kind == ret
}

// AssignUse implements regalloc.Instr AssignUse.
func (r *regAllocInstrImpl) AssignUse(i int, v regalloc.VReg) {
	r.i.assignUse(i, v)
}

// AssignDef implements regalloc.Instr AssignDef.
func (r *regAllocInstrImpl) AssignDef(v regalloc.VReg) {
	r.i.assignDef(v)
}

func (m *machine) insertStoreRegisterAt(v regalloc.VReg, instr *instruction, after bool) {
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.compiler.TypeOf(v)

	offsetFromSP := m.getVRegSpillSlotOffset(v.ID(), typ.Size()) + m.clobberedRegSlotSize()
	m.pendingInstructions = m.pendingInstructions[:0]
	admode := m.resolveAddressModeForOffset(offsetFromSP, typ.Bits(), spVReg)
	store := m.allocateInstrAfterLowering()
	store.asStore(operandNR(v), admode, typ.Bits())

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	// If the offset is large, we might end up with having multiple instructions inserted in resolveAddressModeForOffset.
	for _, instr := range m.pendingInstructions {
		instr.addedAfterLowering = true
		cur.next = instr
		instr.prev = cur
		cur = instr
	}

	cur.next = store
	store.prev = cur

	store.next = prevNext
	prevNext.prev = store
}

func (m *machine) reloadRegister(v regalloc.VReg, instr *instruction, after bool) {
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.compiler.TypeOf(v)

	offsetFromSP := m.getVRegSpillSlotOffset(v.ID(), typ.Size()) + m.clobberedRegSlotSize()
	m.pendingInstructions = m.pendingInstructions[:0]
	admode := m.resolveAddressModeForOffset(offsetFromSP, typ.Bits(), spVReg)
	load := m.allocateInstrAfterLowering()
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		load.asULoad(operandNR(v), admode, typ.Bits())
	case ssa.TypeF32, ssa.TypeF64:
		load.asFpuLoad(operandNR(v), admode, typ.Bits())
	case ssa.TypeV128:
		load.asFpuLoad(operandNR(v), admode, 128)
	default:
		panic("TODO")
	}

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	// If the offset is large, we might end up with having multiple instructions inserted in resolveAddressModeForOffset.
	for _, instr := range m.pendingInstructions {
		instr.addedAfterLowering = true
		cur.next = instr
		instr.prev = cur
		cur = instr
	}

	cur.next = load
	load.prev = cur

	load.next = prevNext
	prevNext.prev = load
}
