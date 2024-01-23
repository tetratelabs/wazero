package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

var addendsMatchOpcodes = [5]ssa.Opcode{ssa.OpcodeUExtend, ssa.OpcodeSExtend, ssa.OpcodeIadd, ssa.OpcodeIconst}

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	offBase := int32(offsetBase)
	def := m.c.ValueDefinition(ptr)
	if op := m.c.MatchInstrOneOf(def, addendsMatchOpcodes[:]); op == ssa.OpcodeIadd {
		x, y := def.Instr.Arg2()
		xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
		rx, offx := m.lowerAddend(xDef)
		ry, offy := m.lowerAddend(yDef)
		return m.lowerAddendsToAmode(rx, ry, offx, offy, offBase)
	} else {
		// If it is not an Iadd, then we lower the one addend.
		r, off := m.lowerAddend(def)
		// off is always 0 if r is valid.
		if r != regalloc.VRegInvalid {
			return newAmodeImmReg(offsetBase, r)
		} else {
			off64 := off + int64(offBase)
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, uint64(off64), true)
			return newAmodeImmReg(0, tmpReg)
		}
	}
}

func (m *machine) lowerAddendsToAmode(rx, ry regalloc.VReg, offx, offy int64, offBase int32) amode {
	if rx != regalloc.VRegInvalid && offx != 0 || ry != regalloc.VRegInvalid && offy != 0 {
		panic("invalid input")
	}
	u64 := uint64(int64(offBase) + offx + offy)
	if u64 != 0 && !lower32willSignExtendTo64(u64) {
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, u64, true)
		// Blank u64 as it has been already lowered.
		u64 = 0
		// We already know that either rx or ry is invalid,
		// so we overwrite it with the temporary register.
		if rx == regalloc.VRegInvalid {
			rx = tmpReg
		} else {
			ry = tmpReg
		}
	}

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
}

func (m *machine) lowerAddend(x *backend.SSAValueDefinition) (regalloc.VReg, int64) {
	if x.IsFromBlockParam() {
		return x.BlkParamVReg, 0
	}
	return m.lowerAddendFromInstr(x.Instr)
}

// lowerAddendFromInstr takes an instruction returns a Vreg and an offset that can be used in an address mode.
// The Vreg is regalloc.VRegInvalid if the addend cannot be lowered to a register.
// The offset is 0 if the addend can be lowered to a register.
func (m *machine) lowerAddendFromInstr(instr *ssa.Instruction) (regalloc.VReg, int64) {
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
		input := instr.Arg()
		inputDef := m.c.ValueDefinition(input)
		switch input.Type().Bits() {
		case 64:
			r := m.getOperand_Reg(inputDef).r
			instr.MarkLowered()
			return r, 0
		case 32:
			constInst := inputDef.IsFromInstr() && inputDef.Instr.Constant()
			switch {
			case constInst && op == ssa.OpcodeSExtend:
				instr.MarkLowered()
				return regalloc.VRegInvalid, int64(uint32(inputDef.Instr.ConstantVal()))
			case constInst && op == ssa.OpcodeUExtend:
				instr.MarkLowered()
				return regalloc.VRegInvalid, int64(int32(inputDef.Instr.ConstantVal())) // sign-extend!
			default:
				r := m.getOperand_Reg(inputDef).r
				instr.MarkLowered()
				return r, 0
			}
		}
	}
	panic("BUG: invalid opcode")
}
