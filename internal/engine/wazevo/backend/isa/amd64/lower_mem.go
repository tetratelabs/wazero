package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

var addendsMatchOpcodes = [5]ssa.Opcode{ssa.OpcodeUExtend, ssa.OpcodeSExtend, ssa.OpcodeIadd, ssa.OpcodeIconst}

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	if !lower32willSignExtendTo64(uint64(offsetBase)) {
		// If the offset is too large to be absorbed into the amode, we just load the offset into a register.
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, uint64(offsetBase), true)
		return newAmodeImmReg(0, tmpReg)
	}

	offBase := int32(offsetBase)
	def := m.c.ValueDefinition(ptr)
	if op := m.c.MatchInstrOneOf(def, addendsMatchOpcodes[:]); op == ssa.OpcodeIadd {
		x, y := def.Instr.Arg2()
		xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
		rx, offx := m.lowerAddend(xDef.Instr)
		ry, offy := m.lowerAddend(yDef.Instr)

		u64 := uint64(int64(offBase) + offx + offy)
		if u64 == 0 || lower32willSignExtendTo64(u64) {
			u32 := uint32(u64)
			switch {
			// We assume rx, ry are valid iff offx, offy are 0.
			case rx != regalloc.VRegInvalid && ry != regalloc.VRegInvalid:
				return newAmodeRegRegShift(u32, rx, ry, 0)
			case rx != regalloc.VRegInvalid && ry == regalloc.VRegInvalid:
				return newAmodeImmReg(u32, rx)
			case rx == regalloc.VRegInvalid && ry != regalloc.VRegInvalid:
				return newAmodeImmReg(u32, ry)
			default: // Both are invalid: use the offset.
				// TODO: if offset == 0, xor vreg, vreg.
				tmpReg := m.c.AllocateVReg(ssa.TypeI64)
				m.lowerIconst(tmpReg, u64, true)
				return newAmodeImmReg(0, tmpReg)
			}
		} else {
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, u64, true)
			return newAmodeImmReg(0, tmpReg)
		}
	} else {
		// If it is not an Iadd, then we lower the one addend.
		r, off := m.lowerAddend(def.Instr)
		off += int64(offBase)
		if r != regalloc.VRegInvalid && lower32willSignExtendTo64(uint64(off)) {
			return newAmodeImmReg(uint32(off), r)
		} else {
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, uint64(off), true)
			return newAmodeImmReg(0, tmpReg)
		}
	}
}

// lowerAddend takes an instruction returns a Vreg and an offset that can be used in an address mode.
// The Vreg is regalloc.VRegInvalid if the addend cannot be lowered to a register.
// The offset is 0 if the addend can be lowered to a register.
func (m *machine) lowerAddend(instr *ssa.Instruction) (regalloc.VReg, int64) {
	switch op := instr.Opcode(); op {
	case ssa.OpcodeIconst:
		instr.MarkLowered()
		u64 := instr.ConstantVal()
		if instr.Return().Type().Bits() == 32 {
			return regalloc.VRegInvalid, int64(int32(u64)) // sign-extend.
		} else {
			return regalloc.VRegInvalid, int64(u64)
		}
	case ssa.OpcodeUExtend, ssa.OpcodeSExtend:
		switch input := instr.Arg(); input.Type().Bits() {
		case 64:
			r := m.getOperand_Reg(m.c.ValueDefinition(input)).r
			instr.MarkLowered()
			return r, 0
		case 32:
			inputDef := m.c.ValueDefinition(input)
			constInst := inputDef.IsFromInstr() && inputDef.Instr.Constant()
			switch {
			case constInst && op == ssa.OpcodeSExtend:
				instr.MarkLowered()
				return regalloc.VRegInvalid, int64(uint32(inputDef.Instr.ConstantVal()))
			case constInst && op == ssa.OpcodeUExtend:
				instr.MarkLowered()
				return regalloc.VRegInvalid, int64(int32(inputDef.Instr.ConstantVal())) // sign-extend!
			}
		}
	}
	panic("BUG: invalid opcode")
}
