package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

var addendsMatchOpcodes = [5]ssa.Opcode{ssa.OpcodeUExtend, ssa.OpcodeSExtend, ssa.OpcodeIadd, ssa.OpcodeIconst, ssa.OpcodeIshl}

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	if !lower32willSignExtendTo64(uint64(offsetBase)) {
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, uint64(offsetBase), true)
		return newAmodeImmReg(0, tmpReg)
	}

	offBase := int32(offsetBase)
	def := m.c.ValueDefinition(ptr)
	switch op := m.c.MatchInstrOneOf(def, addendsMatchOpcodes[:]); op {
	case ssa.OpcodeIadd:
		x, y := def.Instr.Arg2()
		xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
		rx, offx := m.lowerAddend(xDef.Instr)
		ry, offy := m.lowerAddend(yDef.Instr)

		imm32 := uint32(offBase + offx + offy)
		if imm32 != 0 && lower32willSignExtendTo64(uint64(imm32)) {
			switch {
			case rx != regalloc.VRegInvalid && ry != regalloc.VRegInvalid:
				return newAmodeRegRegShift(imm32, rx, ry, 0)
			case rx != regalloc.VRegInvalid && ry == regalloc.VRegInvalid:
				return newAmodeImmReg(imm32, rx)
			case rx == regalloc.VRegInvalid && ry != regalloc.VRegInvalid:
				return newAmodeImmReg(imm32, ry)
			case rx == regalloc.VRegInvalid && ry == regalloc.VRegInvalid:
				tmpReg := m.c.AllocateVReg(ssa.TypeI64)
				m.lowerIconst(tmpReg, uint64(offBase+offx+offy), true)
				return newAmodeImmReg(0, tmpReg)
			}
		} else {
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, uint64(offBase), true)
			return newAmodeImmReg(0, tmpReg)
		}

	default:
		r, off := m.lowerAddend(def.Instr)
		return newAmodeImmReg(uint32(off), r)
	}

}

func (m *machine) lowerAddend(instr *ssa.Instruction) (regalloc.VReg, int32) {
	switch op := instr.Opcode(); op {
	case ssa.OpcodeIconst:
		instr.MarkLowered()
		u64 := instr.ConstantVal()
		if instr.Return().Type().Bits() == 32 || lower32willSignExtendTo64(u64) {
			return regalloc.VRegInvalid, int32(u64) // sign-extend.
		} else {
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, u64, true)
			return tmpReg, 0
		}
	case ssa.OpcodeUExtend, ssa.OpcodeSExtend:
		switch input := instr.Arg(); input.Type().Bits() {
		case 64:
			// If the input is already 64-bit, this extend is a no-op.
			r := m.getOperand_Reg(m.c.ValueDefinition(input)).r
			instr.MarkLowered()
			return r, 0
		case 32:
			inputDef := m.c.ValueDefinition(input)
			constInst := inputDef.IsFromInstr() && inputDef.Instr.Constant()
			switch {
			case constInst && op == ssa.OpcodeSExtend:
				offset := int32(inputDef.Instr.ConstantVal())
				instr.MarkLowered()
				return regalloc.VRegInvalid, offset
			case constInst && op == ssa.OpcodeUExtend:
				instr.MarkLowered()
				u64 := inputDef.Instr.ConstantVal()
				if lower32willSignExtendTo64(u64) {
					// The value is small enough to fit in an i32 anyway.
					return regalloc.VRegInvalid, int32(u64)
				} else {
					tmpReg := m.c.AllocateVReg(ssa.TypeI64)
					m.lowerIconst(tmpReg, u64, true)
					return tmpReg, 0
				}
			}
		}
	case ssa.OpcodeIshl:

	}

	return regalloc.VRegInvalid, 0
}
