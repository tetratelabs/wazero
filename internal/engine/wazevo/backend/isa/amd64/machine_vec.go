package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

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

	maskTableLabelIndex := m.i8x16LogicalSHRMaskTableIndex
	if maskTableLabelIndex < 0 {
		label := m.allocateLabel()
		maskTableLabelIndex = len(m.consts)
		m.consts = append(m.consts, _const{
			_var:  i8x16LogicalSHRMaskTable[:],
			label: label,
		})
		m.i8x16LogicalSHRMaskTableIndex = maskTableLabelIndex
	}

	base := m.c.AllocateVReg(ssa.TypeI64)
	lea := m.allocateInstr().asLEA(newOperandLabel(m.consts[maskTableLabelIndex].label.L), base)
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
		maskTableLabelIndex := m.i8x16SHLMaskTableIndex
		if maskTableLabelIndex < 0 {
			label := m.allocateLabel()
			maskTableLabelIndex = len(m.consts)
			m.consts = append(m.consts, _const{
				_var:  i8x16SHLMaskTable[:],
				label: label,
			})
			m.i8x16SHLMaskTableIndex = maskTableLabelIndex
		}

		base := m.c.AllocateVReg(ssa.TypeI64)
		lea := m.allocateInstr().asLEA(newOperandLabel(m.consts[maskTableLabelIndex].label.L), base)
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
