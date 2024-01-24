package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

var addendsMatchOpcodes = [...]ssa.Opcode{ssa.OpcodeUExtend, ssa.OpcodeSExtend, ssa.OpcodeIadd, ssa.OpcodeIconst, ssa.OpcodeIshl}

type (
	addend struct {
		r     regalloc.VReg
		off   int64
		shift byte
	}
)

// lowerToAddressMode converts a pointer to an addressMode that can be used as an operand for load/store instructions.
func (m *machine) lowerToAddressMode(ptr ssa.Value, offsetBase uint32) (am amode) {
	offBase := int32(offsetBase)
	def := m.c.ValueDefinition(ptr)
	if op := m.c.MatchInstrOneOf(def, addendsMatchOpcodes[:]); op == ssa.OpcodeIadd {
		x, y := def.Instr.Arg2()
		xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
		ax := m.lowerAddend(xDef)
		ay := m.lowerAddend(yDef)
		return m.lowerAddendsToAmode(ax, ay, offBase)
	} else {
		// If it is not an Iadd, then we lower the one addend.
		a := m.lowerAddend(def)
		// off is always 0 if r is valid.
		if a.r != regalloc.VRegInvalid {
			if a.shift != 0 {
				tmpReg := m.c.AllocateVReg(ssa.TypeI64)
				m.lowerIconst(tmpReg, 0, true)
				return newAmodeRegRegShift(offsetBase, tmpReg, a.r, a.shift)
			}
			return newAmodeImmReg(offsetBase, a.r)
		} else {
			off64 := a.off + int64(offBase)
			tmpReg := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(tmpReg, uint64(off64), true)
			return newAmodeImmReg(0, tmpReg)
		}
	}
}

func (m *machine) lowerAddendsToAmode(x, y addend, offBase int32) amode {
	if x.r != regalloc.VRegInvalid && x.off != 0 || y.r != regalloc.VRegInvalid && y.off != 0 {
		panic("invalid input")
	}
	u64 := uint64(int64(offBase) + x.off + y.off)
	if u64 != 0 && !lower32willSignExtendTo64(u64) {
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, u64, true)
		// Blank u64 as it has been already lowered.
		u64 = 0
		// We already know that either rx or ry is invalid,
		// so we overwrite it with the temporary register.
		if x.r == regalloc.VRegInvalid {
			x.r = tmpReg
		} else {
			y.r = tmpReg
		}
	}

	u32 := uint32(u64)
	switch {
	// We assume rx, ry are valid iff offx, offy are 0.
	case x.r != regalloc.VRegInvalid && y.r != regalloc.VRegInvalid:
		switch {
		case x.shift != 0 && y.shift != 0:
			// Cannot absorb two shifted registers, must lower one to a shift instruction.
			shifted := m.allocateInstr()
			shifted.asShiftR(shiftROpShiftLeft, newOperandImm32(uint32(x.shift)), x.r, true)
			m.insert(shifted)

			return newAmodeRegRegShift(u32, x.r, y.r, y.shift)
		case x.shift != 0 && y.shift == 0:
			// Swap base and index.
			x, y = y, x
			fallthrough
		default:
			return newAmodeRegRegShift(u32, x.r, y.r, y.shift)
		}
	case x.r == regalloc.VRegInvalid && y.r != regalloc.VRegInvalid:
		x, y = y, x
		fallthrough
	case x.r != regalloc.VRegInvalid && y.r == regalloc.VRegInvalid:
		if x.shift != 0 {
			zero := m.c.AllocateVReg(ssa.TypeI64)
			m.lowerIconst(zero, 0, true)
			return newAmodeRegRegShift(u32, zero, x.r, x.shift)
		}
		return newAmodeImmReg(u32, x.r)
	default: // Both are invalid: use the offset.
		tmpReg := m.c.AllocateVReg(ssa.TypeI64)
		m.lowerIconst(tmpReg, u64, true)
		return newAmodeImmReg(0, tmpReg)
	}
}

func (m *machine) lowerAddend(x *backend.SSAValueDefinition) addend {
	if x.IsFromBlockParam() {
		return addend{x.BlkParamVReg, 0, 0}
	}
	// Ensure the addend is not referenced in multiple places; we will discard nested Iadds.
	op := m.c.MatchInstrOneOf(x, addendsMatchOpcodes[:])
	if op != ssa.OpcodeInvalid && op != ssa.OpcodeIadd {
		return m.lowerAddendFromInstr(x.Instr)
	}
	return addend{m.getOperand_Reg(x).r, 0, 0}
}

// lowerAddendFromInstr takes an instruction returns a Vreg and an offset that can be used in an address mode.
// The Vreg is regalloc.VRegInvalid if the addend cannot be lowered to a register.
// The offset is 0 if the addend can be lowered to a register.
func (m *machine) lowerAddendFromInstr(instr *ssa.Instruction) addend {
	instr.MarkLowered()
	switch op := instr.Opcode(); op {
	case ssa.OpcodeIconst:
		u64 := instr.ConstantVal()
		if instr.Return().Type().Bits() == 32 {
			return addend{regalloc.VRegInvalid, int64(int32(u64)), 0} // sign-extend.
		} else {
			return addend{regalloc.VRegInvalid, int64(u64), 0}
		}
	case ssa.OpcodeUExtend, ssa.OpcodeSExtend:
		input := instr.Arg()
		inputDef := m.c.ValueDefinition(input)
		if input.Type().Bits() != 32 {
			panic("BUG: invalid input type " + input.Type().String())
		}
		constInst := inputDef.IsFromInstr() && inputDef.Instr.Constant()
		switch {
		case constInst && op == ssa.OpcodeSExtend:
			return addend{regalloc.VRegInvalid, int64(uint32(inputDef.Instr.ConstantVal())), 0}
		case constInst && op == ssa.OpcodeUExtend:
			return addend{regalloc.VRegInvalid, int64(int32(inputDef.Instr.ConstantVal())), 0} // sign-extend!
		default:
			return addend{m.getOperand_Reg(inputDef).r, 0, 0}
		}
	case ssa.OpcodeIshl:
		// If the addend is a shift, we can only handle it if the shift amount is a constant.
		x, amount := instr.Arg2()
		amountDef := m.c.ValueDefinition(amount)
		if amountDef.IsFromInstr() && amountDef.Instr.Constant() && amountDef.Instr.ConstantVal() <= 3 {
			return addend{m.getOperand_Reg(m.c.ValueDefinition(x)).r, 0, uint8(amountDef.Instr.ConstantVal())}
		}
		return addend{m.getOperand_Reg(m.c.ValueDefinition(x)).r, 0, 0}
	}
	panic("BUG: invalid opcode")
}
