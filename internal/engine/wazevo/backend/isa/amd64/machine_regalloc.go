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
	// TODO implement me
	panic("implement me")
}

// InsertReloadRegisterAt implements backend.RegAllocFunctionMachine.
func (m *machine) InsertReloadRegisterAt(v regalloc.VReg, instr *instruction, after bool) *instruction {
	// TODO implement me
	panic("implement me")
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
	// TODO implement me
	panic("implement me")
}
