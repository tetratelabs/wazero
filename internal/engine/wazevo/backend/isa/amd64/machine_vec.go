package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

var swizzleMask = [16]byte{
	0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70,
	0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70,
}

func (m *machine) lowerSwizzle(x, y ssa.Value, ret ssa.Value) {
	masklabel := m.getOrAllocateConstLabel(&m.constSwizzleMaskConstIndex, swizzleMask[:])

	// Load mask to maskReg.
	maskReg := m.c.AllocateVReg(ssa.TypeV128)
	loadMask := m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(masklabel)), maskReg)
	m.insert(loadMask)

	// Copy x and y to tmp registers.
	xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	tmpDst := m.copyToTmp(xx.reg())
	yy := m.getOperand_Reg(m.c.ValueDefinition(y))
	tmpX := m.copyToTmp(yy.reg())

	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePaddusb, newOperandReg(maskReg), tmpX))
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePshufb, newOperandReg(tmpX), tmpDst))

	// Copy the result to the destination register.
	m.copyTo(tmpDst, m.c.VRegOf(ret))
}

func (m *machine) lowerInsertLane(x, y ssa.Value, index byte, ret ssa.Value, lane ssa.VecLane) {
	// Copy x to tmp.
	tmpDst := m.c.AllocateVReg(ssa.TypeV128)
	m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, m.getOperand_Mem_Reg(m.c.ValueDefinition(x)), tmpDst))

	yy := m.getOperand_Reg(m.c.ValueDefinition(y))
	switch lane {
	case ssa.VecLaneI8x16:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrb, index, yy, tmpDst))
	case ssa.VecLaneI16x8:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrw, index, yy, tmpDst))
	case ssa.VecLaneI32x4:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrd, index, yy, tmpDst))
	case ssa.VecLaneI64x2:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrq, index, yy, tmpDst))
	case ssa.VecLaneF32x4:
		// In INSERTPS instruction, the destination index is encoded at 4 and 5 bits of the argument.
		// See https://www.felixcloutier.com/x86/insertps
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeInsertps, index<<4, yy, tmpDst))
	case ssa.VecLaneF64x2:
		if index == 0 {
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovsd, yy, tmpDst))
		} else {
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeMovlhps, yy, tmpDst))
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	m.copyTo(tmpDst, m.c.VRegOf(ret))
}

func (m *machine) lowerExtractLane(x ssa.Value, index byte, signed bool, ret ssa.Value, lane ssa.VecLane) {
	// Pextr variants are used to extract a lane from a vector register.
	xx := m.getOperand_Reg(m.c.ValueDefinition(x))

	tmpDst := m.c.AllocateVReg(ret.Type())
	m.insert(m.allocateInstr().asDefineUninitializedReg(tmpDst))
	switch lane {
	case ssa.VecLaneI8x16:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrb, index, xx, tmpDst))
		if signed {
			m.insert(m.allocateInstr().asMovsxRmR(extModeBL, newOperandReg(tmpDst), tmpDst))
		} else {
			m.insert(m.allocateInstr().asMovzxRmR(extModeBL, newOperandReg(tmpDst), tmpDst))
		}
	case ssa.VecLaneI16x8:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrw, index, xx, tmpDst))
		if signed {
			m.insert(m.allocateInstr().asMovsxRmR(extModeWL, newOperandReg(tmpDst), tmpDst))
		} else {
			m.insert(m.allocateInstr().asMovzxRmR(extModeWL, newOperandReg(tmpDst), tmpDst))
		}
	case ssa.VecLaneI32x4:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrd, index, xx, tmpDst))
	case ssa.VecLaneI64x2:
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrq, index, xx, tmpDst))
	case ssa.VecLaneF32x4:
		if index == 0 {
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovss, xx, tmpDst))
		} else {
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePshufd, index, xx, tmpDst))
		}
	case ssa.VecLaneF64x2:
		if index == 0 {
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovsd, xx, tmpDst))
		} else {
			m.copyTo(xx.reg(), tmpDst)
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePshufd, 0b00_00_11_10, newOperandReg(tmpDst), tmpDst))
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	m.copyTo(tmpDst, m.c.VRegOf(ret))
}

var sqmulRoundSat = [16]byte{
	0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80,
	0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80,
}

func (m *machine) lowerSqmulRoundSat(x, y, ret ssa.Value) {
	// See https://github.com/WebAssembly/simd/pull/365 for the following logic.
	maskLabel := m.getOrAllocateConstLabel(&m.constSqmulRoundSatIndex, sqmulRoundSat[:])

	tmp := m.c.AllocateVReg(ssa.TypeV128)
	loadMask := m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskLabel)), tmp)
	m.insert(loadMask)

	xx, yy := m.getOperand_Reg(m.c.ValueDefinition(x)), m.getOperand_Mem_Reg(m.c.ValueDefinition(y))
	tmpX := m.copyToTmp(xx.reg())

	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePmulhrsw, yy, tmpX))
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePcmpeqd, newOperandReg(tmpX), tmp))
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePxor, newOperandReg(tmp), tmpX))

	m.copyTo(tmpX, m.c.VRegOf(ret))
}

func (m *machine) lowerVUshr(x, y, ret ssa.Value, lane ssa.VecLane) {
	switch lane {
	case ssa.VecLaneI8x16:
		m.lowerVUshri8x16(x, y, ret)
	case ssa.VecLaneI16x8, ssa.VecLaneI32x4, ssa.VecLaneI64x2:
		m.lowerShr(x, y, ret, lane, false)
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}
}

// i8x16LogicalSHRMaskTable is necessary for emulating non-existent packed bytes logical right shifts on amd64.
// The mask is applied after performing packed word shifts on the value to clear out the unnecessary bits.
var i8x16LogicalSHRMaskTable = [8 * 16]byte{ // (the number of possible shift amount 0, 1, ..., 7.) * 16 bytes.
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // for 0 shift
	0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, // for 1 shift
	0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, 0x3f, // for 2 shift
	0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, // for 3 shift
	0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, // for 4 shift
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, // for 5 shift
	0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, // for 6 shift
	0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, // for 7 shift
}

func (m *machine) lowerVUshri8x16(x, y, ret ssa.Value) {
	tmpGpReg := m.c.AllocateVReg(ssa.TypeI32)
	// Load the modulo 8 mask to tmpReg.
	m.lowerIconst(tmpGpReg, 0x7, false)
	// Take the modulo 8 of the shift amount.
	shiftAmt := m.getOperand_Mem_Imm32_Reg(m.c.ValueDefinition(y))
	m.insert(m.allocateInstr().asAluRmiR(aluRmiROpcodeAnd, shiftAmt, tmpGpReg, false))

	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())

	vecTmp := m.c.AllocateVReg(ssa.TypeV128)
	m.insert(m.allocateInstr().asGprToXmm(sseOpcodeMovd, newOperandReg(tmpGpReg), vecTmp, false))
	m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsrlw, newOperandReg(vecTmp), xx))

	maskTableLabel := m.getOrAllocateConstLabel(&m.constI8x16LogicalSHRMaskTableIndex, i8x16LogicalSHRMaskTable[:])
	base := m.c.AllocateVReg(ssa.TypeI64)
	lea := m.allocateInstr().asLEA(newOperandLabel(maskTableLabel), base)
	m.insert(lea)

	// Shift tmpGpReg by 4 to multiply the shift amount by 16.
	m.insert(m.allocateInstr().asShiftR(shiftROpShiftLeft, newOperandImm32(4), tmpGpReg, false))

	mem := m.newAmodeRegRegShift(0, base, tmpGpReg, 0)
	loadMask := m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(mem), vecTmp)
	m.insert(loadMask)

	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePand, newOperandReg(vecTmp), xx))
	m.copyTo(xx, m.c.VRegOf(ret))
}

func (m *machine) lowerVSshr(x, y, ret ssa.Value, lane ssa.VecLane) {
	switch lane {
	case ssa.VecLaneI8x16:
		m.lowerVSshri8x16(x, y, ret)
	case ssa.VecLaneI16x8, ssa.VecLaneI32x4:
		m.lowerShr(x, y, ret, lane, true)
	case ssa.VecLaneI64x2:
		m.lowerVSshri64x2(x, y, ret)
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}
}

func (m *machine) lowerVSshri8x16(x, y, ret ssa.Value) {
	shiftAmtReg := m.c.AllocateVReg(ssa.TypeI32)
	// Load the modulo 8 mask to tmpReg.
	m.lowerIconst(shiftAmtReg, 0x7, false)
	// Take the modulo 8 of the shift amount.
	shiftAmt := m.getOperand_Mem_Imm32_Reg(m.c.ValueDefinition(y))
	m.insert(m.allocateInstr().asAluRmiR(aluRmiROpcodeAnd, shiftAmt, shiftAmtReg, false))

	// Copy the x value to two temporary registers.
	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())
	vecTmp := m.c.AllocateVReg(ssa.TypeV128)
	m.copyTo(xx, vecTmp)

	// Assuming that we have
	//  xx   = [b1, ..., b16]
	//  vecTmp = [b1, ..., b16]
	// at this point, then we use PUNPCKLBW and PUNPCKHBW to produce:
	//  xx   = [b1, b1, b2, b2, ..., b8, b8]
	//  vecTmp = [b9, b9, b10, b10, ..., b16, b16]
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePunpcklbw, newOperandReg(xx), xx))
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePunpckhbw, newOperandReg(vecTmp), vecTmp))

	// Adding 8 to the shift amount, and then move the amount to vecTmp2.
	vecTmp2 := m.c.AllocateVReg(ssa.TypeV128)
	m.insert(m.allocateInstr().asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(8), shiftAmtReg, false))
	m.insert(m.allocateInstr().asGprToXmm(sseOpcodeMovd, newOperandReg(shiftAmtReg), vecTmp2, false))

	// Perform the word packed arithmetic right shifts on vreg and vecTmp.
	// This changes these two registers as:
	//  xx   = [xxx, b1 >> s, xxx, b2 >> s, ..., xxx, b8 >> s]
	//  vecTmp = [xxx, b9 >> s, xxx, b10 >> s, ..., xxx, b16 >> s]
	// where xxx is 1 or 0 depending on each byte's sign, and ">>" is the arithmetic shift on a byte.
	m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsraw, newOperandReg(vecTmp2), xx))
	m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsraw, newOperandReg(vecTmp2), vecTmp))

	// Finally, we can get the result by packing these two word vectors.
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePacksswb, newOperandReg(vecTmp), xx))

	m.copyTo(xx, m.c.VRegOf(ret))
}

func (m *machine) lowerVSshri64x2(x, y, ret ssa.Value) {
	// Load the shift amount to RCX.
	shiftAmt := m.getOperand_Mem_Reg(m.c.ValueDefinition(y))
	m.insert(m.allocateInstr().asMovzxRmR(extModeBQ, shiftAmt, rcxVReg))

	tmpGp := m.c.AllocateVReg(ssa.TypeI64)

	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xxReg := m.copyToTmp(_xx.reg())

	m.insert(m.allocateInstr().asDefineUninitializedReg(tmpGp))
	m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrq, 0, newOperandReg(xxReg), tmpGp))
	m.insert(m.allocateInstr().asShiftR(shiftROpShiftRightArithmetic, newOperandReg(rcxVReg), tmpGp, true))
	m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrq, 0, newOperandReg(tmpGp), xxReg))
	m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePextrq, 1, newOperandReg(xxReg), tmpGp))
	m.insert(m.allocateInstr().asShiftR(shiftROpShiftRightArithmetic, newOperandReg(rcxVReg), tmpGp, true))
	m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrq, 1, newOperandReg(tmpGp), xxReg))

	m.copyTo(xxReg, m.c.VRegOf(ret))
}

func (m *machine) lowerShr(x, y, ret ssa.Value, lane ssa.VecLane, signed bool) {
	var modulo uint64
	var shiftOp sseOpcode
	switch lane {
	case ssa.VecLaneI16x8:
		modulo = 0xf
		if signed {
			shiftOp = sseOpcodePsraw
		} else {
			shiftOp = sseOpcodePsrlw
		}
	case ssa.VecLaneI32x4:
		modulo = 0x1f
		if signed {
			shiftOp = sseOpcodePsrad
		} else {
			shiftOp = sseOpcodePsrld
		}
	case ssa.VecLaneI64x2:
		modulo = 0x3f
		if signed {
			panic("BUG")
		}
		shiftOp = sseOpcodePsrlq
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())

	tmpGpReg := m.c.AllocateVReg(ssa.TypeI32)
	// Load the modulo 8 mask to tmpReg.
	m.lowerIconst(tmpGpReg, modulo, false)
	// Take the modulo 8 of the shift amount.
	m.insert(m.allocateInstr().asAluRmiR(aluRmiROpcodeAnd,
		m.getOperand_Mem_Imm32_Reg(m.c.ValueDefinition(y)), tmpGpReg, false))
	// And move it to a xmm register.
	tmpVec := m.c.AllocateVReg(ssa.TypeV128)
	m.insert(m.allocateInstr().asGprToXmm(sseOpcodeMovd, newOperandReg(tmpGpReg), tmpVec, false))

	// Then do the actual shift.
	m.insert(m.allocateInstr().asXmmRmiReg(shiftOp, newOperandReg(tmpVec), xx))

	m.copyTo(xx, m.c.VRegOf(ret))
}

func (m *machine) lowerVIshl(x, y, ret ssa.Value, lane ssa.VecLane) {
	var modulo uint64
	var shiftOp sseOpcode
	var isI8x16 bool
	switch lane {
	case ssa.VecLaneI8x16:
		isI8x16 = true
		modulo = 0x7
		shiftOp = sseOpcodePsllw
	case ssa.VecLaneI16x8:
		modulo = 0xf
		shiftOp = sseOpcodePsllw
	case ssa.VecLaneI32x4:
		modulo = 0x1f
		shiftOp = sseOpcodePslld
	case ssa.VecLaneI64x2:
		modulo = 0x3f
		shiftOp = sseOpcodePsllq
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())

	tmpGpReg := m.c.AllocateVReg(ssa.TypeI32)
	// Load the modulo 8 mask to tmpReg.
	m.lowerIconst(tmpGpReg, modulo, false)
	// Take the modulo 8 of the shift amount.
	m.insert(m.allocateInstr().asAluRmiR(aluRmiROpcodeAnd,
		m.getOperand_Mem_Imm32_Reg(m.c.ValueDefinition(y)), tmpGpReg, false))
	// And move it to a xmm register.
	tmpVec := m.c.AllocateVReg(ssa.TypeV128)
	m.insert(m.allocateInstr().asGprToXmm(sseOpcodeMovd, newOperandReg(tmpGpReg), tmpVec, false))

	// Then do the actual shift.
	m.insert(m.allocateInstr().asXmmRmiReg(shiftOp, newOperandReg(tmpVec), xx))

	if isI8x16 {
		maskTableLabel := m.getOrAllocateConstLabel(&m.constI8x16SHLMaskTableIndex, i8x16SHLMaskTable[:])
		base := m.c.AllocateVReg(ssa.TypeI64)
		lea := m.allocateInstr().asLEA(newOperandLabel(maskTableLabel), base)
		m.insert(lea)

		// Shift tmpGpReg by 4 to multiply the shift amount by 16.
		m.insert(m.allocateInstr().asShiftR(shiftROpShiftLeft, newOperandImm32(4), tmpGpReg, false))

		mem := m.newAmodeRegRegShift(0, base, tmpGpReg, 0)
		loadMask := m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(mem), tmpVec)
		m.insert(loadMask)

		m.insert(m.allocateInstr().asXmmRmR(sseOpcodePand, newOperandReg(tmpVec), xx))
	}

	m.copyTo(xx, m.c.VRegOf(ret))
}

// i8x16SHLMaskTable is necessary for emulating non-existent packed bytes left shifts on amd64.
// The mask is applied after performing packed word shifts on the value to clear out the unnecessary bits.
var i8x16SHLMaskTable = [8 * 16]byte{ // (the number of possible shift amount 0, 1, ..., 7.) * 16 bytes.
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // for 0 shift
	0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, 0xfe, // for 1 shift
	0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc, // for 2 shift
	0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, 0xf8, // for 3 shift
	0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, // for 4 shift
	0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, 0xe0, // for 5 shift
	0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, 0xc0, // for 6 shift
	0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, // for 7 shift
}

func (m *machine) lowerVRound(x, ret ssa.Value, imm byte, _64 bool) {
	xx := m.getOperand_Mem_Reg(m.c.ValueDefinition(x))
	var round sseOpcode
	if _64 {
		round = sseOpcodeRoundpd
	} else {
		round = sseOpcodeRoundps
	}
	m.insert(m.allocateInstr().asXmmUnaryRmRImm(round, imm, xx, m.c.VRegOf(ret)))
}

func (m *machine) lowerIaddPairwise(x, y, ret ssa.Value, lane ssa.VecLane) {
	xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	tmp := m.copyToTmp(xx.reg())

	var sseOp sseOpcode
	switch lane {
	case ssa.VecLaneI8x16:
		sseOp = sseOpcodePaddb
	case ssa.VecLaneI16x8:
		sseOp = sseOpcodePaddw
	case ssa.VecLaneI32x4:
		sseOp = sseOpcodePaddd
	case ssa.VecLaneI64x2:
		sseOp = sseOpcodePaddq
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	aaddw := m.allocateInstr().asXmmRmR(sseOp, m.getOperand_Mem_Reg(m.c.ValueDefinition(y)), tmp)
	m.insert(aaddw)

	m.copyTo(tmp, m.c.VRegOf(ret))
}

func (m *machine) lowerWidenLow(x, ret ssa.Value, lane ssa.VecLane, signed bool) {
	var sseOp sseOpcode
	switch lane {
	case ssa.VecLaneI8x16:
		if signed {
			sseOp = sseOpcodePmovsxbw
		} else {
			sseOp = sseOpcodePmovzxbw
		}
	case ssa.VecLaneI16x8:
		if signed {
			sseOp = sseOpcodePmovsxwd
		} else {
			sseOp = sseOpcodePmovzxwd
		}
	case ssa.VecLaneI32x4:
		if signed {
			sseOp = sseOpcodePmovsxdq
		} else {
			sseOp = sseOpcodePmovzxdq
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	xx := m.getOperand_Mem_Reg(m.c.ValueDefinition(x))
	m.insert(m.allocateInstr().asXmmUnaryRmR(sseOp, xx, m.c.VRegOf(ret)))
}

func (m *machine) lowerWidenHigh(x, ret ssa.Value, lane ssa.VecLane, signed bool) {
	tmp := m.c.AllocateVReg(ssa.TypeV128)
	xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	m.copyTo(xx.reg(), tmp)
	m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePalignr, 8, newOperandReg(tmp), tmp))

	var sseOp sseOpcode
	switch lane {
	case ssa.VecLaneI8x16:
		if signed {
			sseOp = sseOpcodePmovsxbw
		} else {
			sseOp = sseOpcodePmovzxbw
		}
	case ssa.VecLaneI16x8:
		if signed {
			sseOp = sseOpcodePmovsxwd
		} else {
			sseOp = sseOpcodePmovzxwd
		}
	case ssa.VecLaneI32x4:
		if signed {
			sseOp = sseOpcodePmovsxdq
		} else {
			sseOp = sseOpcodePmovzxdq
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	m.insert(m.allocateInstr().asXmmUnaryRmR(sseOp, newOperandReg(tmp), m.c.VRegOf(ret)))
}

func (m *machine) lowerLoadSplat(ptr ssa.Value, offset uint32, ret ssa.Value, lane ssa.VecLane) {
	tmpDst, tmpGp := m.c.AllocateVReg(ssa.TypeV128), m.c.AllocateVReg(ssa.TypeI64)
	am := newOperandMem(m.lowerToAddressMode(ptr, offset))

	m.insert(m.allocateInstr().asDefineUninitializedReg(tmpDst))
	switch lane {
	case ssa.VecLaneI8x16:
		m.insert(m.allocateInstr().asMovzxRmR(extModeBQ, am, tmpGp))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrb, 0, newOperandReg(tmpGp), tmpDst))
		tmpZeroVec := m.c.AllocateVReg(ssa.TypeV128)
		m.insert(m.allocateInstr().asZeros(tmpZeroVec))
		m.insert(m.allocateInstr().asXmmRmR(sseOpcodePshufb, newOperandReg(tmpZeroVec), tmpDst))
	case ssa.VecLaneI16x8:
		m.insert(m.allocateInstr().asMovzxRmR(extModeWQ, am, tmpGp))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrw, 0, newOperandReg(tmpGp), tmpDst))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrw, 1, newOperandReg(tmpGp), tmpDst))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePshufd, 0, newOperandReg(tmpDst), tmpDst))
	case ssa.VecLaneI32x4:
		m.insert(m.allocateInstr().asMovzxRmR(extModeLQ, am, tmpGp))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrd, 0, newOperandReg(tmpGp), tmpDst))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePshufd, 0, newOperandReg(tmpDst), tmpDst))
	case ssa.VecLaneI64x2:
		m.insert(m.allocateInstr().asMov64MR(am, tmpGp))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrq, 0, newOperandReg(tmpGp), tmpDst))
		m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodePinsrq, 1, newOperandReg(tmpGp), tmpDst))
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	m.copyTo(tmpDst, m.c.VRegOf(ret))
}

var f64x2CvtFromIMask = [16]byte{
	0x00, 0x00, 0x30, 0x43, 0x00, 0x00, 0x30, 0x43, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func (m *machine) lowerVFcvtFromInt(x, ret ssa.Value, lane ssa.VecLane, signed bool) {
	switch lane {
	case ssa.VecLaneF32x4:
		if signed {
			xx := m.getOperand_Mem_Reg(m.c.ValueDefinition(x))
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvtdq2ps, xx, m.c.VRegOf(ret)))
		} else {
			xx := m.getOperand_Reg(m.c.ValueDefinition(x))
			// Copy the value to two temporary registers.
			tmp := m.copyToTmp(xx.reg())
			tmp2 := m.copyToTmp(xx.reg())

			// Clear the higher 16 bits of each 32-bit element.
			m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePslld, newOperandImm32(0xa), tmp))
			m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsrld, newOperandImm32(0xa), tmp))

			// Subtract the higher 16-bits from tmp2: clear the lower 16-bits of tmp2.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePsubd, newOperandReg(tmp), tmp2))

			// Convert the lower 16-bits in tmp.
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvtdq2ps, newOperandReg(tmp), tmp))

			// Left shift by one and convert tmp2, meaning that halved conversion result of higher 16-bits in tmp2.
			m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsrld, newOperandImm32(1), tmp2))
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvtdq2ps, newOperandReg(tmp2), tmp2))

			// Double the converted halved higher 16bits.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAddps, newOperandReg(tmp2), tmp2))

			// Get the conversion result by add tmp (holding lower 16-bit conversion) into tmp2.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAddps, newOperandReg(tmp), tmp2))

			m.copyTo(tmp2, m.c.VRegOf(ret))
		}
	case ssa.VecLaneF64x2:
		if signed {
			xx := m.getOperand_Mem_Reg(m.c.ValueDefinition(x))
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvtdq2pd, xx, m.c.VRegOf(ret)))
		} else {
			maskReg := m.c.AllocateVReg(ssa.TypeV128)
			maskLabel := m.getOrAllocateConstLabel(&m.constF64x2CvtFromIMaskIndex, f64x2CvtFromIMask[:])
			// maskReg = [0x00, 0x00, 0x30, 0x43, 0x00, 0x00, 0x30, 0x43, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskLabel)), maskReg))

			_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
			xx := m.copyToTmp(_xx.reg())

			// Given that we have xx = [d1, d2, d3, d4], this results in
			//	xx = [d1, [0x00, 0x00, 0x30, 0x43], d2, [0x00, 0x00, 0x30, 0x43]]
			//     = [float64(uint32(d1)) + 0x1.0p52, float64(uint32(d2)) + 0x1.0p52]
			//     ^See https://stackoverflow.com/questions/13269523/can-all-32-bit-ints-be-exactly-represented-as-a-double
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeUnpcklps, newOperandReg(maskReg), xx))

			// maskReg = [float64(0x1.0p52), float64(0x1.0p52)]
			maskLabel = m.getOrAllocateConstLabel(&m.constTwop52Index, twop52[:])
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskLabel)), maskReg))

			// Now, we get the result as
			// 	xx = [float64(uint32(d1)), float64(uint32(d2))]
			// because the following equality always satisfies:
			//  float64(0x1.0p52 + float64(uint32(x))) - float64(0x1.0p52 + float64(uint32(y))) = float64(uint32(x)) - float64(uint32(y))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeSubpd, newOperandReg(maskReg), xx))

			m.copyTo(xx, m.c.VRegOf(ret))
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}
}

var (
	// i32sMaxOnF64x2 holds math.MaxInt32(=2147483647.0) on two f64 lanes.
	i32sMaxOnF64x2 = [16]byte{
		0x00, 0x00, 0xc0, 0xff, 0xff, 0xff, 0xdf, 0x41, // float64(2147483647.0)
		0x00, 0x00, 0xc0, 0xff, 0xff, 0xff, 0xdf, 0x41, // float64(2147483647.0)
	}

	// i32sMaxOnF64x2 holds math.MaxUint32(=4294967295.0) on two f64 lanes.
	i32uMaxOnF64x2 = [16]byte{
		0x00, 0x00, 0xe0, 0xff, 0xff, 0xff, 0xef, 0x41, // float64(4294967295.0)
		0x00, 0x00, 0xe0, 0xff, 0xff, 0xff, 0xef, 0x41, // float64(4294967295.0)
	}

	// twop52 holds two float64(0x1.0p52) on two f64 lanes. 0x1.0p52 is special in the sense that
	// with this exponent, the mantissa represents a corresponding uint32 number, and arithmetics,
	// like addition or subtraction, the resulted floating point holds exactly the same
	// bit representations in 32-bit integer on its mantissa.
	//
	// Note: the name twop52 is common across various compiler ecosystem.
	// 	E.g. https://github.com/llvm/llvm-project/blob/92ab024f81e5b64e258b7c3baaf213c7c26fcf40/compiler-rt/lib/builtins/floatdidf.c#L28
	// 	E.g. https://opensource.apple.com/source/clang/clang-425.0.24/src/projects/compiler-rt/lib/floatdidf.c.auto.html
	twop52 = [16]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x43, // float64(0x1.0p52)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x43, // float64(0x1.0p52)
	}
)

func (m *machine) lowerVFcvtToIntSat(x, ret ssa.Value, lane ssa.VecLane, signed bool) {
	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())

	switch lane {
	case ssa.VecLaneF32x4:
		if signed {
			tmp := m.copyToTmp(xx)

			// Assuming we have xx = [v1, v2, v3, v4].
			//
			// Set all bits if lane is not NaN on tmp.
			// tmp[i] = 0xffffffff  if vi != NaN
			//        = 0           if vi == NaN
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_OQ), newOperandReg(tmp), tmp))

			// Clear NaN lanes on xx, meaning that
			// 	xx[i] = vi  if vi != NaN
			//	        0   if vi == NaN
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAndps, newOperandReg(tmp), xx))

			// tmp[i] = ^vi         if vi != NaN
			//        = 0xffffffff  if vi == NaN
			// which means that tmp[i] & 0x80000000 != 0 if and only if vi is negative.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeXorps, newOperandReg(xx), tmp))

			// xx[i] = int32(vi)   if vi != NaN and xx is not overflowing.
			//       = 0x80000000  if vi != NaN and xx is overflowing (See https://www.felixcloutier.com/x86/cvttps2dq)
			//       = 0           if vi == NaN
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvttps2dq, newOperandReg(xx), xx))

			// Below, we have to convert 0x80000000 into 0x7FFFFFFF for positive overflowing lane.
			//
			// tmp[i] = 0x80000000                         if vi is positive
			//        = any satisfying any&0x80000000 = 0  if vi is negative or zero.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAndps, newOperandReg(xx), tmp))

			// Arithmetic right shifting tmp by 31, meaning that we have
			// tmp[i] = 0xffffffff if vi is positive, 0 otherwise.
			m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsrad, newOperandImm32(0x1f), tmp))

			// Flipping 0x80000000 if vi is positive, otherwise keep intact.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePxor, newOperandReg(tmp), xx))
		} else {
			tmp := m.c.AllocateVReg(ssa.TypeV128)
			m.insert(m.allocateInstr().asZeros(tmp))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeMaxps, newOperandReg(tmp), xx))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePcmpeqd, newOperandReg(tmp), tmp))
			m.insert(m.allocateInstr().asXmmRmiReg(sseOpcodePsrld, newOperandImm32(0x1), tmp))
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvtdq2ps, newOperandReg(tmp), tmp))
			tmp2 := m.copyToTmp(xx)
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvttps2dq, newOperandReg(xx), xx))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeSubps, newOperandReg(tmp), tmp2))
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLE_OS), newOperandReg(tmp2), tmp))
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvttps2dq, newOperandReg(tmp2), tmp2))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePxor, newOperandReg(tmp), tmp2))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePxor, newOperandReg(tmp), tmp))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePmaxsd, newOperandReg(tmp), tmp2))
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodePaddd, newOperandReg(tmp2), xx))
		}

	case ssa.VecLaneF64x2:
		tmp2 := m.c.AllocateVReg(ssa.TypeV128)
		if signed {
			tmp := m.copyToTmp(xx)

			// Set all bits for non-NaN lanes, zeros otherwise.
			// I.e. tmp[i] = 0xffffffff_ffffffff if vi != NaN, 0 otherwise.
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_OQ), newOperandReg(tmp), tmp))

			maskLabel := m.getOrAllocateConstLabel(&m.constI32sMaxOnF64x2Index, i32sMaxOnF64x2[:])
			// Load the 2147483647 into tmp2's each lane.
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskLabel)), tmp2))

			// tmp[i] = 2147483647 if vi != NaN, 0 otherwise.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAndps, newOperandReg(tmp2), tmp))

			// MINPD returns the source register's value as-is, so we have
			//  xx[i] = vi   if vi != NaN
			//        = 0    if vi == NaN
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeMinpd, newOperandReg(tmp), xx))

			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeCvttpd2dq, newOperandReg(xx), xx))
		} else {
			tmp := m.c.AllocateVReg(ssa.TypeV128)
			m.insert(m.allocateInstr().asZeros(tmp))

			//  xx[i] = vi   if vi != NaN && vi > 0
			//        = 0    if vi == NaN || vi <= 0
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeMaxpd, newOperandReg(tmp), xx))

			// tmp2[i] = float64(math.MaxUint32) = math.MaxUint32
			maskIndex := m.getOrAllocateConstLabel(&m.constI32uMaxOnF64x2Index, i32uMaxOnF64x2[:])
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskIndex)), tmp2))

			// xx[i] = vi   if vi != NaN && vi > 0 && vi <= math.MaxUint32
			//       = 0    otherwise
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeMinpd, newOperandReg(tmp2), xx))

			// Round the floating points into integer.
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeRoundpd, 0x3, newOperandReg(xx), xx))

			// tmp2[i] = float64(0x1.0p52)
			maskIndex = m.getOrAllocateConstLabel(&m.constTwop52Index, twop52[:])
			m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(m.newAmodeRipRel(maskIndex)), tmp2))

			// xx[i] = float64(0x1.0p52) + float64(uint32(vi)) if vi != NaN && vi > 0 && vi <= math.MaxUint32
			//       = 0                                       otherwise
			//
			// This means that xx[i] holds exactly the same bit of uint32(vi) in its lower 32-bits.
			m.insert(m.allocateInstr().asXmmRmR(sseOpcodeAddpd, newOperandReg(tmp2), xx))

			// At this point, we have
			// 	xx  = [uint32(v0), float64(0x1.0p52), uint32(v1), float64(0x1.0p52)]
			//  tmp = [0, 0, 0, 0]
			// as 32x4 lanes. Therefore, SHUFPS with 0b00_00_10_00 results in
			//	xx = [xx[00], xx[10], tmp[00], tmp[00]] = [xx[00], xx[10], 0, 0]
			// meaning that for i = 0 and 1, we have
			//  xx[i] = uint32(vi) if vi != NaN && vi > 0 && vi <= math.MaxUint32
			//        = 0          otherwise.
			m.insert(m.allocateInstr().asXmmRmRImm(sseOpcodeShufps, 0b00_00_10_00, newOperandReg(tmp), xx))
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}

	m.copyTo(xx, m.c.VRegOf(ret))
}

func (m *machine) lowerNarrow(x, y, ret ssa.Value, lane ssa.VecLane, signed bool) {
	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())
	yy := m.getOperand_Mem_Reg(m.c.ValueDefinition(y))

	var sseOp sseOpcode
	switch lane {
	case ssa.VecLaneI16x8:
		if signed {
			sseOp = sseOpcodePacksswb
		} else {
			sseOp = sseOpcodePackuswb
		}
	case ssa.VecLaneI32x4:
		if signed {
			sseOp = sseOpcodePackssdw
		} else {
			sseOp = sseOpcodePackusdw
		}
	default:
		panic(fmt.Sprintf("invalid lane type: %s", lane))
	}
	m.insert(m.allocateInstr().asXmmRmR(sseOp, yy, xx))
	m.copyTo(xx, m.c.VRegOf(ret))
}

func (m *machine) lowerWideningPairwiseDotProductS(x, y, ret ssa.Value) {
	_xx := m.getOperand_Reg(m.c.ValueDefinition(x))
	xx := m.copyToTmp(_xx.reg())
	yy := m.getOperand_Mem_Reg(m.c.ValueDefinition(y))
	m.insert(m.allocateInstr().asXmmRmR(sseOpcodePmaddwd, yy, xx))
	m.copyTo(xx, m.c.VRegOf(ret))
}
