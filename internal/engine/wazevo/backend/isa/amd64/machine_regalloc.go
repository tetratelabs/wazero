package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// InsertMoveBefore implements backend.RegAllocFunctionMachine.
func (m *machine) InsertMoveBefore(dst, src regalloc.VReg, instr *instruction) {
	// TODO implement me
	panic("implement me")
}

// InsertStoreRegisterAt implements backend.RegAllocFunctionMachine.
func (m *machine) InsertStoreRegisterAt(v regalloc.VReg, instr *instruction, after bool) *instruction {
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.c.TypeOf(v)

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	offsetFromSP := m.getVRegSpillSlotOffsetFromSP(v.ID(), typ.Size())
	var am amode
	cur, am = m.resolveAddressModeForOffsetAndInsert(cur, offsetFromSP, typ.Bits(), rspVReg)
	store := m.allocateInstr()
	store.asMovRM(v, newOperandMem(am), typ.Bits()/8)

	cur = linkInstr(cur, store)
	return linkInstr(cur, prevNext)
}

// InsertReloadRegisterAt implements backend.RegAllocFunctionMachine.
func (m *machine) InsertReloadRegisterAt(v regalloc.VReg, instr *instruction, after bool) *instruction {
	// TODO implement me
	if !v.IsRealReg() {
		panic("BUG: VReg must be backed by real reg to be stored")
	}

	typ := m.c.TypeOf(v)

	var prevNext, cur *instruction
	if after {
		cur, prevNext = instr, instr.next
	} else {
		cur, prevNext = instr.prev, instr
	}

	offsetFromSP := m.getVRegSpillSlotOffsetFromSP(v.ID(), typ.Size())
	var am amode
	cur, am = m.resolveAddressModeForOffsetAndInsert(cur, offsetFromSP, typ.Bits(), rspVReg)
	load := m.allocateInstr()
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		load.asLEA(am, v)
	// case ssa.TypeF32, ssa.TypeF64:
	//	load.asFpuLoad(operandNR(v), amode, typ.Bits())
	// case ssa.TypeV128:
	//	load.asFpuLoad(operandNR(v), amode, 128)
	default:
		panic("TODO")
	}

	cur = linkInstr(cur, load)
	return linkInstr(cur, prevNext)
}

// ClobberedRegisters implements backend.RegAllocFunctionMachine.
func (m *machine) ClobberedRegisters(regs []regalloc.VReg) {
	m.clobberedRegs = append(m.clobberedRegs[:0], regs...)
}

// Swap implements backend.RegAllocFunctionMachine.
func (m *machine) Swap(cur *instruction, x1, x2, tmp regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// LastInstrForInsertion implements backend.RegAllocFunctionMachine.
func (m *machine) LastInstrForInsertion(begin, end *instruction) *instruction {
	// TODO implement me
	panic("implement me")
}

// SSABlockLabel implements backend.RegAllocFunctionMachine.
func (m *machine) SSABlockLabel(id ssa.BasicBlockID) backend.Label {
	return m.ectx.SsaBlockIDToLabels[id]
}
