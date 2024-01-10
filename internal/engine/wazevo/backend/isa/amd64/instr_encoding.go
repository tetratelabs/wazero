package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// Encode implements backend.Machine Encode.
func (m *machine) Encode() {
	m.encode(m.ectx.RootInstr)
}

func (m *machine) encode(root *instruction) {
	for cur := root; cur != nil; cur = cur.next {
		cur.encode(m.c)
	}
}

func (i *instruction) encode(c backend.Compiler) {
	switch i.kind {
	case nop0:
	case ret:
		c.EmitByte(0xc3)
	case imm:
		dst := regEncodings[i.op1.r.RealReg()]
		con := i.u1
		if i.b1 { // 64 bit.
			if lower32willSignExtendTo64(con) {
				// Sign extend mov(imm32).
				encodeRegReg(c,
					legacyPrefixesNone,
					0xc7, 1,
					0,
					dst,
					rexInfo(0).setW(),
				)
				c.Emit4Bytes(uint32(con))
			} else {
				c.EmitByte(rexEncodingW | dst.rexBit())
				c.EmitByte(0xb8 | dst.encoding())
				c.Emit8Bytes(con)
			}
		} else {
			if dst.rexBit() > 0 {
				c.EmitByte(rexEncodingDefault | 0x1)
			}
			c.EmitByte(0xb8 | dst.encoding())
			c.Emit4Bytes(uint32(con))
		}

	case aluRmiR:
		var rex rexInfo
		if i.b1 {
			rex = rex.setW()
		} else {
			rex = rex.clearW()
		}

		dst := regEncodings[i.op2.r.RealReg()]

		aluOp := aluRmiROpcode(i.u1)
		if aluOp == aluRmiROpcodeMul {
			op1 := i.op1
			const regMemOpc, regMemOpcNum = 0x0FAF, 2
			switch op1.kind {
			case operandKindReg:
				src := regEncodings[op1.r.RealReg()]
				encodeRegReg(c, legacyPrefixesNone, regMemOpc, regMemOpcNum, dst, src, rex)
			case operandKindMem:
				m := i.op1.amode
				encodeRegMem(c, legacyPrefixesNone, regMemOpc, regMemOpcNum, dst, m, rex)
			case operandImm32:
				imm8 := lower8willSignExtendTo32(op1.imm32)
				var opc uint32
				if imm8 {
					opc = 0x6b
				} else {
					opc = 0x69
				}
				encodeRegReg(c, legacyPrefixesNone, opc, 1, dst, dst, rex)
				if imm8 {
					c.EmitByte(byte(op1.imm32))
				} else {
					c.Emit4Bytes(op1.imm32)
				}
			}
		} else {
			const opcodeNum = 1
			var opcR, opcM, subOpcImm uint32
			switch aluOp {
			case aluRmiROpcodeAdd:
				opcR, opcM, subOpcImm = 0x01, 0x03, 0x0
			case aluRmiROpcodeSub:
				opcR, opcM, subOpcImm = 0x29, 0x2b, 0x5
			case aluRmiROpcodeAnd:
				opcR, opcM, subOpcImm = 0x21, 0x23, 0x4
			case aluRmiROpcodeOr:
				opcR, opcM, subOpcImm = 0x09, 0x0b, 0x1
			case aluRmiROpcodeXor:
				opcR, opcM, subOpcImm = 0x31, 0x33, 0x6
			default:
				panic("BUG: invalid aluRmiROpcode")
			}

			op1 := i.op1
			switch op1.kind {
			case operandKindReg:
				src := regEncodings[op1.r.RealReg()]
				encodeRegReg(c, legacyPrefixesNone, opcR, opcodeNum, src, dst, rex)
			case operandKindMem:
				m := i.op1.amode
				encodeRegMem(c, legacyPrefixesNone, opcM, opcodeNum, dst, m, rex)
			case operandImm32:
				imm8 := lower8willSignExtendTo32(op1.imm32)
				var opc uint32
				if imm8 {
					opc = 0x83
				} else {
					opc = 0x81
				}
				encodeRegReg(c, legacyPrefixesNone, opc, opcodeNum, regEnc(subOpcImm), dst, rex)
				if imm8 {
					c.EmitByte(byte(op1.imm32))
				} else {
					c.Emit4Bytes(op1.imm32)
				}
			}
		}

	case movRR:
		src := regEncodings[i.op1.r.RealReg()]
		dst := regEncodings[i.op2.r.RealReg()]
		var rex rexInfo
		if i.b1 {
			rex = rex.setW()
		} else {
			rex = rex.clearW()
		}
		encodeRegReg(c, legacyPrefixesNone, 0x89, 1, src, dst, rex)

	case xmmRmR:
		op := sseOpcode(i.u1)
		var legPrex legacyPrefixes
		var opcode uint32
		var opcodeNum uint32
		switch op {
		case sseOpcodeAddps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F58, 2
		case sseOpcodeAddpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F58, 2
		case sseOpcodeAddss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F58, 2
		case sseOpcodeAddsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F58, 2
		case sseOpcodeAndps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F54, 2
		case sseOpcodeAndpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F54, 2
		case sseOpcodeAndnps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F55, 2
		case sseOpcodeAndnpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F55, 2
		case sseOpcodeCvttps2dq:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5B, 2
		case sseOpcodeCvtdq2ps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F5B, 2
		case sseOpcodeDivps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F5E, 2
		case sseOpcodeDivpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F5E, 2
		case sseOpcodeDivss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5E, 2
		case sseOpcodeDivsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F5E, 2
		case sseOpcodeMaxps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F5F, 2
		case sseOpcodeMaxpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F5F, 2
		case sseOpcodeMaxss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5F, 2
		case sseOpcodeMaxsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F5F, 2
		case sseOpcodeMinps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F5D, 2
		case sseOpcodeMinpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F5D, 2
		case sseOpcodeMinss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5D, 2
		case sseOpcodeMinsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F5D, 2
		case sseOpcodeMovlhps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F16, 2
		case sseOpcodeMovsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F10, 2
		case sseOpcodeMulps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F59, 2
		case sseOpcodeMulpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F59, 2
		case sseOpcodeMulss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F59, 2
		case sseOpcodeMulsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F59, 2
		case sseOpcodeOrpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F56, 2
		case sseOpcodeOrps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F56, 2
		case sseOpcodePackssdw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F6B, 2
		case sseOpcodePacksswb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F63, 2
		case sseOpcodePackusdw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F382B, 3
		case sseOpcodePackuswb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F67, 2
		case sseOpcodePaddb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FFC, 2
		case sseOpcodePaddd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FFE, 2
		case sseOpcodePaddq:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FD4, 2
		case sseOpcodePaddw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FFD, 2
		case sseOpcodePaddsb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FEC, 2
		case sseOpcodePaddsw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FED, 2
		case sseOpcodePaddusb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDC, 2
		case sseOpcodePaddusw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDD, 2
		case sseOpcodePand:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDB, 2
		case sseOpcodePandn:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDF, 2
		case sseOpcodePavgb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FE0, 2
		case sseOpcodePavgw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FE3, 2
		case sseOpcodePcmpeqb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F74, 2
		case sseOpcodePcmpeqw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F75, 2
		case sseOpcodePcmpeqd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F76, 2
		case sseOpcodePcmpeqq:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3829, 3
		case sseOpcodePcmpgtb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F64, 2
		case sseOpcodePcmpgtw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F65, 2
		case sseOpcodePcmpgtd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F66, 2
		case sseOpcodePcmpgtq:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3837, 3
		case sseOpcodePmaddwd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FF5, 2
		case sseOpcodePmaxsb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383C, 3
		case sseOpcodePmaxsw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FEE, 2
		case sseOpcodePmaxsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383D, 3
		case sseOpcodePmaxub:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDE, 2
		case sseOpcodePmaxuw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383E, 3
		case sseOpcodePmaxud:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383F, 3
		case sseOpcodePminsb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3838, 3
		case sseOpcodePminsw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FEA, 2
		case sseOpcodePminsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3839, 3
		case sseOpcodePminub:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FDA, 2
		case sseOpcodePminuw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383A, 3
		case sseOpcodePminud:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F383B, 3
		case sseOpcodePmulld:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3840, 3
		case sseOpcodePmullw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FD5, 2
		case sseOpcodePmuludq:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FF4, 2
		case sseOpcodePor:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FEB, 2
		case sseOpcodePshufb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3800, 3
		case sseOpcodePsubb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FF8, 2
		case sseOpcodePsubd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FFA, 2
		case sseOpcodePsubq:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FFB, 2
		case sseOpcodePsubw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FF9, 2
		case sseOpcodePsubsb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FE8, 2
		case sseOpcodePsubsw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FE9, 2
		case sseOpcodePsubusb:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FD8, 2
		case sseOpcodePsubusw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FD9, 2
		case sseOpcodePunpckhbw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F68, 2
		case sseOpcodePunpcklbw:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F60, 2
		case sseOpcodePxor:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0FEF, 2
		case sseOpcodeSubps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F5C, 2
		case sseOpcodeSubpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F5C, 2
		case sseOpcodeSubss:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5C, 2
		case sseOpcodeSubsd:
			legPrex, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F5C, 2
		case sseOpcodeXorps:
			legPrex, opcode, opcodeNum = legacyPrefixesNone, 0x0F57, 2
		case sseOpcodeXorpd:
			legPrex, opcode, opcodeNum = legacyPrefixes0x66, 0x0F57, 2
		default:
			panic(fmt.Sprintf("Unsupported sseOpcode: %s", op))
		}

		dst := regEncodings[i.op2.r.RealReg()]

		rex := rexInfo(0).clearW()
		op1 := i.op1
		if op1.kind == operandKindReg {
			src := regEncodings[op1.r.RealReg()]
			encodeRegReg(c, legPrex, opcode, opcodeNum, dst, src, rex)
		} else if i.op1.kind == operandKindMem {
			m := i.op1.amode
			encodeRegMem(c, legPrex, opcode, opcodeNum, dst, m, rex)
		} else {
			panic("BUG: invalid operand kind")
		}

	case gprToXmm:
		var legPrefix legacyPrefixes
		var opcode uint32
		const opcodeNum = 2
		switch sseOpcode(i.u1) {
		case sseOpcodeMovd, sseOpcodeMovq:
			legPrefix, opcode = legacyPrefixes0x66, 0x0f6e
		case sseOpcodeCvtsi2ss:
			legPrefix, opcode = legacyPrefixes0xF3, 0x0f2a
		case sseOpcodeCvtsi2sd:
			legPrefix, opcode = legacyPrefixes0xF2, 0x0f2a
		default:
			panic(fmt.Sprintf("Unsupported sseOpcode: %s", sseOpcode(i.u1)))
		}

		var rex rexInfo
		if i.b1 {
			rex = rex.setW()
		} else {
			rex = rex.clearW()
		}
		dst := regEncodings[i.op2.r.RealReg()]

		op1 := i.op1
		if op1.kind == operandKindReg {
			src := regEncodings[op1.r.RealReg()]
			encodeRegReg(c, legPrefix, opcode, opcodeNum, dst, src, rex)
		} else if i.op1.kind == operandKindMem {
			m := i.op1.amode
			encodeRegMem(c, legPrefix, opcode, opcodeNum, dst, m, rex)
		} else {
			panic("BUG: invalid operand kind")
		}

	case xmmUnaryRmR:
		var prefix legacyPrefixes
		var opcode uint32
		var opcodeNum uint32
		op := sseOpcode(i.u1)
		switch op {
		case sseOpcodeCvtss2sd:
			prefix, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F5A, 2
		case sseOpcodeCvtsd2ss:
			prefix, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F5A, 2
		case sseOpcodeMovaps:
			prefix, opcode, opcodeNum = legacyPrefixesNone, 0x0F28, 2
		case sseOpcodeMovapd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F28, 2
		case sseOpcodeMovdqa:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F6F, 2
		case sseOpcodeMovdqu:
			prefix, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F6F, 2
		case sseOpcodeMovsd:
			prefix, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F10, 2
		case sseOpcodeMovss:
			prefix, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F10, 2
		case sseOpcodeMovups:
			prefix, opcode, opcodeNum = legacyPrefixesNone, 0x0F10, 2
		case sseOpcodeMovupd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F10, 2
		case sseOpcodePabsb:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F381C, 3
		case sseOpcodePabsw:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F381D, 3
		case sseOpcodePabsd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F381E, 3
		case sseOpcodePmovsxbd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3821, 3
		case sseOpcodePmovsxbw:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3820, 3
		case sseOpcodePmovsxbq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3822, 3
		case sseOpcodePmovsxwd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3823, 3
		case sseOpcodePmovsxwq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3824, 3
		case sseOpcodePmovsxdq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3825, 3
		case sseOpcodePmovzxbd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3831, 3
		case sseOpcodePmovzxbw:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3830, 3
		case sseOpcodePmovzxbq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3832, 3
		case sseOpcodePmovzxwd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3833, 3
		case sseOpcodePmovzxwq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3834, 3
		case sseOpcodePmovzxdq:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F3835, 3
		case sseOpcodeSqrtps:
			prefix, opcode, opcodeNum = legacyPrefixesNone, 0x0F51, 2
		case sseOpcodeSqrtpd:
			prefix, opcode, opcodeNum = legacyPrefixes0x66, 0x0F51, 2
		case sseOpcodeSqrtss:
			prefix, opcode, opcodeNum = legacyPrefixes0xF3, 0x0F51, 2
		case sseOpcodeSqrtsd:
			prefix, opcode, opcodeNum = legacyPrefixes0xF2, 0x0F51, 2
		default:
			panic(fmt.Sprintf("Unsupported sseOpcode: %s", op))
		}

		dst := regEncodings[i.op2.r.RealReg()]

		rex := rexInfo(0).clearW()
		op1 := i.op1
		if op1.kind == operandKindReg {
			src := regEncodings[op1.r.RealReg()]
			encodeRegReg(c, prefix, opcode, opcodeNum, dst, src, rex)
		} else if i.op1.kind == operandKindMem {
			m := i.op1.amode
			encodeRegMem(c, prefix, opcode, opcodeNum, dst, m, rex)
		} else {
			panic("BUG: invalid operand kind")
		}

	case unaryRmR:
		panic("TODO")
	case not:
		panic("TODO")
	case neg:
		panic("TODO")
	case div:
		panic("TODO")
	case mulHi:
		panic("TODO")
	case checkedDivOrRemSeq:
		panic("TODO")
	case signExtendData:
		panic("TODO")
	case movzxRmR:
		panic("TODO")
	case mov64MR:
		panic("TODO")
	case lea:
		a := i.op1.amode
		dst := regEncodings[i.op2.r.RealReg()]
		encodeRegMem(c, legacyPrefixesNone, 0x8d, 1, dst, a, rexInfo(0).setW())

	case movsxRmR:
		panic("TODO")
	case movRM:
		panic("TODO")
	case shiftR:
		panic("TODO")
	case xmmRmiReg:
		panic("TODO")
	case cmpRmiR:
		panic("TODO")
	case setcc:
		panic("TODO")
	case cmove:
		panic("TODO")
	case push64:
		op := i.op1

		switch op.kind {
		case operandKindReg:
			dst := regEncodings[op.r.RealReg()]
			if dst.rexBit() > 0 {
				c.EmitByte(rexEncodingDefault | 0x1)
			}
			c.EmitByte(0x50 | dst.encoding())
		case operandKindMem:
			m := op.amode
			encodeRegMem(
				c, legacyPrefixesNone, 0xff, 1, regEnc(6), m, rexInfo(0).clearW(),
			)
		case operandImm32:
			c.EmitByte(0x68)
			c.Emit4Bytes(op.imm32)
		}

	case pop64:
		dst := regEncodings[i.op1.r.RealReg()]
		if dst.rexBit() > 0 {
			c.EmitByte(rexEncodingDefault | 0x1)
		}
		c.EmitByte(0x58 | dst.encoding())
	case xmmMovRM:
		panic("TODO")
	case xmmLoadConst:
		panic("TODO")
	case xmmToGpr:
		panic("TODO")
	case cvtUint64ToFloatSeq:
		panic("TODO")
	case cvtFloatToSintSeq:
		panic("TODO")
	case cvtFloatToUintSeq:
		panic("TODO")
	case xmmMinMaxSeq:
		panic("TODO")
	case xmmCmove:
		panic("TODO")
	case xmmCmpRmR:
		panic("TODO")
	case xmmRmRImm:
		panic("TODO")
	case jmpKnown:
		panic("TODO")
	case jmpIf:
		panic("TODO")
	case jmpCond:
		panic("TODO")
	case jmpTableSeq:
		panic("TODO")
	case jmpUnknown:
		panic("TODO")
	case trapIf:
		panic("TODO")

	case ud2:
		c.EmitByte(0x0f)
		c.EmitByte(0x0b)

	case call:
		c.EmitByte(0xe8)
		if i.u2 == 0 { // Meaning that the call target is a function value, and requires relocation.
			c.AddRelocationInfo(ssa.FuncRef(i.u1))
		}
		// Note that this is zero as a placeholder for the call target if it's a function value.
		c.Emit4Bytes(uint32(i.u2))

	case callIndirect:
		op := i.op1

		const opcodeNum = 1
		const opcode = 0xff
		rex := rexInfo(0).clearW()
		switch op.kind {
		case operandKindReg:
			dst := regEncodings[op.r.RealReg()]
			encodeRegReg(c,
				legacyPrefixesNone,
				opcode, opcodeNum,
				regEnc(2),
				dst,
				rex,
			)
		case operandKindMem:
			m := op.amode
			encodeRegMem(c,
				legacyPrefixesNone,
				opcode, opcodeNum,
				regEnc(2),
				m,
				rex,
			)
		default:
			panic("BUG: invalid operand kind")
		}

	default:
		panic(fmt.Sprintf("TODO: %v", i.kind))
	}
}

func encodeRegReg(
	c backend.Compiler,
	legPrefixes legacyPrefixes,
	opcodes uint32,
	opcodeNum uint32,
	r regEnc,
	rm regEnc,
	rex rexInfo,
) {
	legPrefixes.encode(c)
	rex.encode(c, r, rm)

	for opcodeNum > 0 {
		opcodeNum--
		c.EmitByte(byte((opcodes >> (opcodeNum << 3)) & 0xff))
	}
	c.EmitByte(encodeModRM(3, r.encoding(), rm.encoding()))
}

func encodeModRM(mod byte, reg byte, rm byte) byte {
	return mod<<6 | reg<<3 | rm
}

func encodeSIB(shift byte, encIndex byte, encBase byte) byte {
	return shift<<6 | encIndex<<3 | encBase
}

func encodeRegMem(
	c backend.Compiler, legPrefixes legacyPrefixes, opcodes uint32, opcodeNum uint32, r regEnc, m amode, rex rexInfo,
) {
	legPrefixes.encode(c)

	const (
		modNoDisplacement    = 0b00
		modShortDisplacement = 0b01
		modLongDisplacement  = 0b10

		useSBI = 4 // the encoding of rsp or r12 register.
	)

	switch m.kind {
	case amodeImmReg:
		base := m.base.RealReg()
		baseEnc := regEncodings[base]

		rex.encode(c, r, baseEnc)

		for opcodeNum > 0 {
			opcodeNum--
			c.EmitByte(byte((opcodes >> (opcodeNum << 3)) & 0xff))
		}

		// SIB byte is the last byte of the memory encoding before the displacement
		const sibByte = 0x24 // == encodeSIB(0, 4, 4)

		immZero, baseRbp, baseR13 := m.imm32 == 0, base == rbp, base == r13
		short := lower8willSignExtendTo32(m.imm32)
		rspOrR12 := base == rsp || base == r12

		if immZero && !baseRbp && !baseR13 { // rbp or r13 can't be used as base for without displacement encoding.
			c.EmitByte(encodeModRM(modNoDisplacement, r.encoding(), baseEnc.encoding()))
			if rspOrR12 {
				c.EmitByte(sibByte)
			}
		} else if short { // Note: this includes the case where m.imm32 == 0 && base == rbp || base == r13.
			c.EmitByte(encodeModRM(modShortDisplacement, r.encoding(), baseEnc.encoding()))
			if rspOrR12 {
				c.EmitByte(sibByte)
			}
			c.EmitByte(byte(m.imm32))
		} else {
			c.EmitByte(encodeModRM(modLongDisplacement, r.encoding(), baseEnc.encoding()))
			if rspOrR12 {
				c.EmitByte(sibByte)
			}
			c.Emit4Bytes(m.imm32)
		}

	case amodeRegRegShift:
		base := m.base.RealReg()
		baseEnc := regEncodings[base]
		index := m.index.RealReg()
		indexEnc := regEncodings[index]

		if index == rsp {
			panic("BUG: rsp can't be used as index of addressing mode")
		}

		rex.encodeForIndex(c, r, indexEnc, baseEnc)

		for opcodeNum > 0 {
			opcodeNum--
			c.EmitByte(byte((opcodes >> (opcodeNum << 3)) & 0xff))
		}

		immZero, baseRbp, baseR13 := m.imm32 == 0, base == rbp, base == r13
		if immZero && !baseRbp && !baseR13 { // rbp or r13 can't be used as base for without displacement encoding. (curious why? because it's interpreted as RIP relative addressing).
			c.EmitByte(encodeModRM(modNoDisplacement, r.encoding(), useSBI))
			c.EmitByte(encodeSIB(m.shift, indexEnc.encoding(), baseEnc.encoding()))
		} else if lower8willSignExtendTo32(m.imm32) {
			c.EmitByte(encodeModRM(modShortDisplacement, r.encoding(), useSBI))
			c.EmitByte(encodeSIB(m.shift, indexEnc.encoding(), baseEnc.encoding()))
			c.EmitByte(byte(m.imm32))
		} else {
			c.EmitByte(encodeModRM(modLongDisplacement, r.encoding(), useSBI))
			c.EmitByte(encodeSIB(m.shift, indexEnc.encoding(), baseEnc.encoding()))
			c.Emit4Bytes(m.imm32)
		}

	case amodeRipRelative:
		if m.label != backend.LabelInvalid {
			panic("BUG: label must be resolved for amodeRipRelative at this point")
		}

		rex.encode(c, r, 0)
		for opcodeNum > 0 {
			opcodeNum--
			c.EmitByte(byte((opcodes >> (opcodeNum << 3)) & 0xff))
		}

		// Indicate "LEAQ [RIP + 32bit displacement].
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#32.2F64-bit_addressing
		c.EmitByte(encodeModRM(0b00, r.encoding(), 0b101))
		c.Emit4Bytes(m.imm32)
	}
}

const (
	rexEncodingDefault byte = 0x40
	rexEncodingW            = rexEncodingDefault | 0x08
)

// rexInfo is a bit set to indicate:
//
//	0x01: W bit must be cleared.
//	0x02: REX prefix must be emitted.
type rexInfo byte

func (ri rexInfo) setW() rexInfo {
	return ri | 0x01
}

func (ri rexInfo) clearW() rexInfo {
	return ri & 0x02
}

func (ri rexInfo) always() rexInfo { //nolint
	return ri | 0x02
}

func (ri rexInfo) notAlways() rexInfo { //nolint
	return ri & 0x01
}

func (ri rexInfo) encode(c backend.Compiler, encR regEnc, encRM regEnc) {
	var w byte = 0
	if ri&0x01 != 0 {
		w = 0x01
	}
	r := encR.rexBit()
	b := encRM.rexBit()
	rex := rexEncodingDefault | w<<3 | r<<2 | b
	if rex != rexEncodingDefault || ri&0x02 == 1 {
		c.EmitByte(rex)
	}
}

func (ri rexInfo) encodeForIndex(c backend.Compiler, encR regEnc, encIndex regEnc, encBase regEnc) {
	var w byte = 0
	if ri&0x01 != 0 {
		w = 0x01
	}
	r := encR.rexBit()
	x := encIndex.rexBit()
	b := encBase.rexBit()
	rex := byte(0x40) | w<<3 | r<<2 | x<<1 | b
	if rex != 0x40 || ri&0x02 == 1 {
		c.EmitByte(rex)
	}
}

type regEnc byte

func (r regEnc) rexBit() byte {
	return byte(r) >> 3
}

func (r regEnc) encoding() byte {
	return byte(r) & 0x07
}

var regEncodings = [...]regEnc{
	rax:   0b000,
	rcx:   0b001,
	rdx:   0b010,
	rbx:   0b011,
	rsp:   0b100,
	rbp:   0b101,
	rsi:   0b110,
	rdi:   0b111,
	r8:    0b1000,
	r9:    0b1001,
	r10:   0b1010,
	r11:   0b1011,
	r12:   0b1100,
	r13:   0b1101,
	r14:   0b1110,
	r15:   0b1111,
	xmm0:  0b000,
	xmm1:  0b001,
	xmm2:  0b010,
	xmm3:  0b011,
	xmm4:  0b100,
	xmm5:  0b101,
	xmm6:  0b110,
	xmm7:  0b111,
	xmm8:  0b1000,
	xmm9:  0b1001,
	xmm10: 0b1010,
	xmm11: 0b1011,
	xmm12: 0b1100,
	xmm13: 0b1101,
	xmm14: 0b1110,
	xmm15: 0b1111,
}

type legacyPrefixes byte

const (
	legacyPrefixesNone legacyPrefixes = iota
	legacyPrefixes0x66
	legacyPrefixes0xF0
	legacyPrefixes0x660xF0
	legacyPrefixes0xF2
	legacyPrefixes0xF3
)

func (p legacyPrefixes) encode(c backend.Compiler) {
	switch p {
	case legacyPrefixesNone:
	case legacyPrefixes0x66:
		c.EmitByte(0x66)
	case legacyPrefixes0xF0:
		c.EmitByte(0xf0)
	case legacyPrefixes0x660xF0:
		c.EmitByte(0x66)
		c.EmitByte(0xf0)
	case legacyPrefixes0xF2:
		c.EmitByte(0xf2)
	case legacyPrefixes0xF3:
		c.EmitByte(0xf3)
	default:
		panic("BUG: invalid legacy prefix")
	}
}

func lower32willSignExtendTo64(x uint64) bool {
	xs := int64(x)
	return xs == int64(uint64(int32(xs)))
}

func lower8willSignExtendTo32(x uint32) bool {
	xs := int32(x)
	return xs == ((xs << 24) >> 24)
}
