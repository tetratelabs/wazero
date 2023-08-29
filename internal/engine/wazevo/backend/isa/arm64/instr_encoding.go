package arm64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// Encode implements backend.Machine Encode.
func (m *machine) Encode() {
	m.encode(m.rootInstr)
}

func (m *machine) encode(root *instruction) {
	for cur := root; cur != nil; cur = cur.next {
		cur.encode(m.compiler)
	}
}

func (i *instruction) encode(c backend.Compiler) {
	switch kind := i.kind; kind {
	case nop0:
	case exitSequence:
		encodeExitSequence(c, i.rn.reg())
	case ret:
		// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/RET--Return-from-subroutine-?lang=en
		c.Emit4Bytes(encodeRet())
	case br:
		imm := i.brOffset()
		c.Emit4Bytes(encodeUnconditionalBranch(false, imm))
	case call:
		if i.u2 > 0 {
			// This is a special case for EmitGoEntryPreamble which doesn't need reloc info,
			// but instead the imm is already resolved.
			c.Emit4Bytes(encodeUnconditionalBranch(true, int64(i.u2)))
		} else {
			// We still don't know the exact address of the function to call, so we emit a placeholder.
			c.AddRelocationInfo(i.callFuncRef())
			c.Emit4Bytes(encodeUnconditionalBranch(true, 0)) // 0 = placeholder
		}
	case callInd:
		c.Emit4Bytes(encodeUnconditionalBranchReg(regNumberInEncoding[i.rn.realReg()], true))
	case store8, store16, store32, store64, fpuStore32, fpuStore64, fpuStore128:
		c.Emit4Bytes(encodeLoadOrStore(i.kind, regNumberInEncoding[i.rn.realReg()], i.amode))
	case uLoad8, uLoad16, uLoad32, uLoad64, sLoad8, sLoad16, sLoad32, fpuLoad32, fpuLoad64, fpuLoad128:
		c.Emit4Bytes(encodeLoadOrStore(i.kind, regNumberInEncoding[i.rd.realReg()], i.amode))
	case condBr:
		imm19 := i.condBrOffset()
		if imm19%4 != 0 {
			panic("imm26 for branch must be a multiple of 4")
		}

		imm19U32 := uint32(imm19/4) & 0b111_11111111_11111111
		brCond := i.condBrCond()
		switch brCond.kind() {
		case condKindRegisterZero:
			rt := regNumberInEncoding[brCond.register().RealReg()]
			c.Emit4Bytes(encodeCBZCBNZ(rt, false, imm19U32, i.condBr64bit()))
		case condKindRegisterNotZero:
			rt := regNumberInEncoding[brCond.register().RealReg()]
			c.Emit4Bytes(encodeCBZCBNZ(rt, true, imm19U32, i.condBr64bit()))
		case condKindCondFlagSet:
			// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/B-cond--Branch-conditionally-
			fl := brCond.flag()
			c.Emit4Bytes(0b01010100<<24 | (imm19U32 << 5) | uint32(fl))
		default:
			panic("BUG")
		}
	case movN:
		c.Emit4Bytes(encodeMoveWideImmediate(0b00, regNumberInEncoding[i.rd.realReg()], i.u1, i.u2, i.u3))
	case movZ:
		c.Emit4Bytes(encodeMoveWideImmediate(0b10, regNumberInEncoding[i.rd.realReg()], i.u1, i.u2, i.u3))
	case movK:
		c.Emit4Bytes(encodeMoveWideImmediate(0b11, regNumberInEncoding[i.rd.realReg()], i.u1, i.u2, i.u3))
	case mov32:
		to, from := i.rd.realReg(), i.rn.realReg()
		c.Emit4Bytes(encodeAsMov32(regNumberInEncoding[from], regNumberInEncoding[to]))
	case mov64:
		to, from := i.rd.realReg(), i.rn.realReg()
		toIsSp := to == sp
		fromIsSp := from == sp
		if toIsSp || fromIsSp {
			// This is an alias of ADD (immediate):
			// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/MOV--to-from-SP---Move-between-register-and-stack-pointer--an-alias-of-ADD--immediate--
			c.Emit4Bytes(encodeAddSubtractImmediate(0b100, 0, 0,
				regNumberInEncoding[from], regNumberInEncoding[to]),
			)
		} else {
			// This is an alias of ORR (shifted register):
			// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/MOV--register---Move--register---an-alias-of-ORR--shifted-register--
			c.Emit4Bytes(encodeLogicalShiftedRegister(0b101, 0, regNumberInEncoding[from], 0, regNumberInEncoding[xzr], regNumberInEncoding[to]))
		}
	case loadP64, storeP64:
		rt, rt2 := regNumberInEncoding[i.rn.realReg()], regNumberInEncoding[i.rm.realReg()]
		amode := i.amode
		rn := regNumberInEncoding[amode.rn.RealReg()]
		var pre bool
		switch amode.kind {
		case addressModeKindPostIndex:
		case addressModeKindPreIndex:
			pre = true
		default:
			panic("BUG")
		}
		c.Emit4Bytes(encodePreOrPostIndexLoadStorePair64(pre, kind == loadP64, rn, rt, rt2, amode.imm))
	case loadFpuConst32:
		encodeLoadFpuConst32(c, regNumberInEncoding[i.rd.realReg()], i.u1)
	case loadFpuConst64:
		encodeLoadFpuConst64(c, regNumberInEncoding[i.rd.realReg()], i.u1)
	case aluRRRR:
		c.Emit4Bytes(encodeAluRRRR(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[i.rm.realReg()],
			regNumberInEncoding[i.ra.realReg()],
			uint32(i.u3),
		))
	case aluRRImmShift:
		c.Emit4Bytes(encodeAluRRImm(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			uint32(i.rm.shiftImm()),
			uint32(i.u3),
		))
	case aluRRR:
		rn := i.rn.realReg()
		c.Emit4Bytes(encodeAluRRR(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[rn],
			regNumberInEncoding[i.rm.realReg()],
			i.u3 == 1,
			rn == sp,
		))
	case aluRRRShift:
		r, amt, sop := i.rm.sr()
		c.Emit4Bytes(encodeAluRRRShift(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[r.RealReg()],
			uint32(amt),
			sop,
			i.u3 == 1,
		))
	case aluRRBitmaskImm:
		c.Emit4Bytes(encodeAluBitmaskImmediate(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			i.u2,
			i.u3 == 1,
		))
	case bitRR:
		c.Emit4Bytes(encodeBitRR(
			bitOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			uint32(i.u2)),
		)
	case aluRRImm12:
		imm12, shift := i.rm.imm12()
		c.Emit4Bytes(encodeAluRRImm12(
			aluOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			imm12, shift,
			i.u3 == 1,
		))
	case fpuRRR:
		c.Emit4Bytes(encodeFpuRRR(
			fpuBinOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[i.rm.realReg()],
			i.u3 == 1,
		))
	case fpuMov64, fpuMov128:
		// https://developer.arm.com/documentation/ddi0596/2021-12/SIMD-FP-Instructions/MOV--vector---Move-vector--an-alias-of-ORR--vector--register--
		rd := regNumberInEncoding[i.rd.realReg()]
		rn := regNumberInEncoding[i.rn.realReg()]
		var q uint32
		if kind == fpuMov128 {
			q = 0b1
		}
		c.Emit4Bytes(q<<30 | 0b1110101<<21 | rn<<16 | 0b000111<<10 | rn<<5 | rd)
	case cSet:
		rd := regNumberInEncoding[i.rd.realReg()]
		cf := condFlag(i.u1)
		// https://developer.arm.com/documentation/ddi0602/2022-06/Base-Instructions/CSET--Conditional-Set--an-alias-of-CSINC-
		// Note that we set 64bit version here.
		c.Emit4Bytes(0b1001101010011111<<16 | uint32(cf.invert())<<12 | 0b111111<<5 | rd)
	case extend:
		c.Emit4Bytes(encodeExtend(i.u3 == 1, byte(i.u1), byte(i.u2), regNumberInEncoding[i.rd.realReg()], regNumberInEncoding[i.rn.realReg()]))
	case fpuCmp:
		// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/FCMP--Floating-point-quiet-Compare--scalar--?lang=en
		rn, rm := regNumberInEncoding[i.rn.realReg()], regNumberInEncoding[i.rm.realReg()]
		var ftype uint32
		if i.u3 == 1 {
			ftype = 0b01 // double precision.
		}
		c.Emit4Bytes(0b1111<<25 | ftype<<22 | 1<<21 | rm<<16 | 0b1<<13 | rn<<5)
	case udf:
		// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/UDF--Permanently-Undefined-?lang=en
		c.Emit4Bytes(0)
	case adr:
		c.Emit4Bytes(encodeAdr(regNumberInEncoding[i.rd.realReg()], uint32(i.u1)))
	case cSel:
		c.Emit4Bytes(encodeConditionalSelect(
			kind,
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[i.rm.realReg()],
			condFlag(i.u1),
			i.u3 == 1,
		))
	case fpuCSel:
		c.Emit4Bytes(encodeFpuCSel(
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[i.rm.realReg()],
			condFlag(i.u1),
			i.u3 == 1,
		))
	case movToVec:
		c.Emit4Bytes(encodeMoveToVec(
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			vecArrangement(byte(i.u1)),
			vecIndex(i.u2),
		))
	case movFromVec:
		c.Emit4Bytes(encodeMoveFromVec(
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			vecArrangement(byte(i.u1)),
			vecIndex(i.u2),
		))
	case vecMisc:
		c.Emit4Bytes(encodeAdvancedSIMDTwoMisc(
			vecOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			vecArrangement(i.u2),
		))

	case vecLanes:
		c.Emit4Bytes(encodeVecLanes(
			vecOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			vecArrangement(i.u2),
		))
	case brTableSequence:
		encodeBrTableSequence(c, i.rn.reg(), i.targets)
	case fpuToInt, intToFpu:
		c.Emit4Bytes(encodeCnvBetweenFloatInt(i))
	case fpuRR:
		c.Emit4Bytes(encodeFloatDataOneSource(
			fpuUniOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			i.u3 == 1,
		))
	case vecRRR:
		c.Emit4Bytes(encodeVecRRR(
			vecOp(i.u1),
			regNumberInEncoding[i.rd.realReg()],
			regNumberInEncoding[i.rn.realReg()],
			regNumberInEncoding[i.rm.realReg()],
			vecArrangement(i.u2),
		))
	case cCmpImm:
		// Conditional compare (immediate) in https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en
		sf := uint32(i.u3 & 0b1)
		nzcv := uint32(i.u2 & 0b1111)
		cond := uint32(condFlag(i.u1))
		imm := uint32(i.rm.data & 0b11111)
		rn := regNumberInEncoding[i.rn.realReg()]
		c.Emit4Bytes(
			sf<<31 | 0b111101001<<22 | imm<<16 | cond<<12 | 0b1<<11 | rn<<5 | nzcv,
		)
	case movFromFPSR:
		rt := regNumberInEncoding[i.rd.realReg()]
		c.Emit4Bytes(encodeSystemRegisterMove(rt, true))
	case movToFPSR:
		rt := regNumberInEncoding[i.rn.realReg()]
		c.Emit4Bytes(encodeSystemRegisterMove(rt, false))
	default:
		panic(i.String())
	}
}

// encodeSystemRegisterMove encodes as "System register move" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Branches--Exception-Generating-and-System-instructions?lang=en
//
// Note that currently we only supports read/write of FPSR.
func encodeSystemRegisterMove(rt uint32, fromSystem bool) uint32 {
	ret := 0b11010101<<24 | 0b11011<<16 | 0b01000100<<8 | 0b001<<5 | rt
	if fromSystem {
		ret |= 0b1 << 21
	}
	return ret
}

// encodeVecRRR encodes as either "Advanced SIMD three *" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeVecRRR(op vecOp, rd, rn, rm uint32, arr vecArrangement) uint32 {
	switch op {
	case vecOpBit:
		var q uint32
		switch arr {
		case vecArrangement8B:
			q = 0b0
		case vecArrangement16B:
			q = 0b1
		default:
			panic("BUG")
		}
		return encodeAdvancedSIMDThreeSame(rd, rn, rm, 0b00011, 0b10, 0b1, q)
	default:
		panic("TODO")
	}
}

// encodeAdvancedSIMDThreeSame encodes as "Advanced SIMD three same" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeAdvancedSIMDThreeSame(rd, rn, rm, opcode, size, U, Q uint32) uint32 {
	return Q<<30 | U<<29 | 0b111<<25 | size<<22 | 0b1<<21 | rm<<16 | opcode<<11 | 0b1<<10 | rn<<5 | rd
}

// encodeFloatDataOneSource encodes as "Floating-point data-processing (1 source)" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en#simd-dp
func encodeFloatDataOneSource(op fpuUniOp, rd, rn uint32, dst64bit bool) uint32 {
	var opcode, ptype uint32
	switch op {
	case fpuUniOpCvt32To64:
		opcode = 0b000101
	case fpuUniOpCvt64To32:
		opcode = 0b000100
		ptype = 0b01
	case fpuUniOpNeg:
		opcode = 0b000010
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpSqrt:
		opcode = 0b000011
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpRoundPlus:
		opcode = 0b001001
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpRoundMinus:
		opcode = 0b001010
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpRoundZero:
		opcode = 0b001011
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpRoundNearest:
		opcode = 0b001000
		if dst64bit {
			ptype = 0b01
		}
	case fpuUniOpAbs:
		opcode = 0b000001
		if dst64bit {
			ptype = 0b01
		}
	default:
		panic("BUG")
	}
	return 0b1111<<25 | ptype<<22 | 0b1<<21 | opcode<<15 | 0b1<<14 | rn<<5 | rd
}

// encodeCnvBetweenFloatInt encodes as "Conversion between floating-point and integer" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeCnvBetweenFloatInt(i *instruction) uint32 {
	rd := regNumberInEncoding[i.rd.realReg()]
	rn := regNumberInEncoding[i.rn.realReg()]

	var opcode uint32
	var rmode uint32
	var ptype uint32
	var sf uint32
	switch i.kind {
	case intToFpu: // Either UCVTF or SCVTF.
		rmode = 0b00

		signed := i.u1 == 1
		src64bit := i.u2 == 1
		dst64bit := i.u3 == 1
		if signed {
			opcode = 0b010
		} else {
			opcode = 0b011
		}
		if src64bit {
			sf = 0b1
		}
		if dst64bit {
			ptype = 0b01
		} else {
			ptype = 0b00
		}
	case fpuToInt: // Either FCVTZU or FCVTZS.
		rmode = 0b11

		signed := i.u1 == 1
		src64bit := i.u2 == 1
		dst64bit := i.u3 == 1

		if signed {
			opcode = 0b000
		} else {
			opcode = 0b001
		}
		if dst64bit {
			sf = 0b1
		}
		if src64bit {
			ptype = 0b01
		} else {
			ptype = 0b00
		}
	}
	return sf<<31 | 0b1111<<25 | ptype<<22 | 0b1<<21 | rmode<<19 | opcode<<16 | rn<<5 | rd
}

// encodeAdr encodes a PC-relative ADR instruction.
// https://developer.arm.com/documentation/ddi0602/2022-06/Base-Instructions/ADR--Form-PC-relative-address-
func encodeAdr(rd uint32, offset uint32) uint32 {
	if offset >= 1<<20 {
		panic("BUG: too large adr instruction")
	}
	return offset&0b11<<29 | 0b1<<28 | offset&0b1111111111_1111111100<<3 | rd
}

// encodeFpuCSel encodes as "Floating-point conditional select" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeFpuCSel(rd, rn, rm uint32, c condFlag, _64bit bool) uint32 {
	var ftype uint32
	if _64bit {
		ftype = 0b01 // double precision.
	}
	return 0b1111<<25 | ftype<<22 | 0b1<<21 | rm<<16 | uint32(c)<<12 | 0b11<<10 | rn<<5 | rd
}

// encodeMoveToVec encodes as "Move general-purpose register to a vector element" (represented as `ins`) in
// https://developer.arm.com/documentation/dui0801/g/A64-SIMD-Vector-Instructions/MOV--vector--from-general-
// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/MOV--from-general---Move-general-purpose-register-to-a-vector-element--an-alias-of-INS--general--?lang=en
func encodeMoveToVec(rd, rn uint32, arr vecArrangement, index vecIndex) uint32 {
	var imm5 uint32
	switch arr {
	case vecArrangementB:
		imm5 |= 0b1
		imm5 |= uint32(index) << 1
		if index > 0b1111 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 15", index))
		}
	case vecArrangementH:
		imm5 |= 0b10
		imm5 |= uint32(index) << 2
		if index > 0b111 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 7", index))
		}
	case vecArrangementS:
		imm5 |= 0b100
		imm5 |= uint32(index) << 3
		if index > 0b11 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 3", index))
		}
	case vecArrangementD:
		imm5 |= 0b1000
		imm5 |= uint32(index) << 4
		if index > 0b1 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 1", index))
		}
	default:
		panic("Unsupported arrangement " + arr.String())
	}

	return 0b01001110000<<21 | imm5<<16 | 0b000111<<10 | rn<<5 | rd
}

// encodeUnconditionalBranchReg encodes as "Unconditional branch (register)" in:
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Branches--Exception-Generating-and-System-instructions?lang=en
func encodeUnconditionalBranchReg(rn uint32, link bool) uint32 {
	var opc uint32
	if link {
		opc = 0b0001
	}
	return 0b1101011<<25 | opc<<21 | 0b11111<<16 | rn<<5
}

// encodeMoveFromVec encodes as "Move vector element to a general-purpose register"
// (represented as `umov` when dest is 32-bit, `umov` otherwise) in
// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/UMOV--Unsigned-Move-vector-element-to-general-purpose-register-?lang=en
// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/MOV--to-general---Move-vector-element-to-general-purpose-register--an-alias-of-UMOV-?lang=en
func encodeMoveFromVec(rd, rn uint32, arr vecArrangement, index vecIndex) uint32 {
	var q uint32
	var imm5 uint32
	switch arr {
	case vecArrangementB:
		imm5 |= 0b1
		imm5 |= uint32(index) << 1
		if index > 0b1111 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 15", index))
		}
	case vecArrangementH:
		imm5 |= 0b10
		imm5 |= uint32(index) << 2
		if index > 0b111 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 7", index))
		}
	case vecArrangementS:
		imm5 |= 0b100
		imm5 |= uint32(index) << 3
		if index > 0b11 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 3", index))
		}
	case vecArrangementD:
		imm5 |= 0b1000
		imm5 |= uint32(index) << 4
		q = 0b1
		if index > 0b1 {
			panic(fmt.Sprintf("vector index is larger than the allowed bound: %d > 1", index))
		}
	default:
		panic("Unsupported arrangement " + arr.String())
	}
	return 0b0_001110000<<21 | q<<30 | imm5<<16 | 0b001111<<10 | rn<<5 | rd
}

// encodeConditionalSelect encodes as "Conditional select" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en#condsel
func encodeConditionalSelect(kind instructionKind, rd, rn, rm uint32, c condFlag, _64bit bool) uint32 {
	if kind != cSel {
		panic("TODO: support other conditional select")
	}

	ret := 0b110101<<23 | rm<<16 | uint32(c)<<12 | rn<<5 | rd
	if _64bit {
		ret |= 0b1 << 31
	}
	return ret
}

const dummyInstruction uint32 = 0x14000000 // "b 0"

// encodeLoadFpuConst32 encodes the following three instructions:
//
//	ldr s8, #8  ;; literal load of data.f32
//	b 8           ;; skip the data
//	data.f32 xxxxxxx
func encodeLoadFpuConst32(c backend.Compiler, rd uint32, rawF32 uint64) {
	c.Emit4Bytes(
		// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/LDR--literal--SIMD-FP---Load-SIMD-FP-Register--PC-relative-literal--?lang=en
		0b111<<26 | (0x8/4)<<5 | rd,
	)
	c.Emit4Bytes(encodeUnconditionalBranch(false, 8)) // b 8
	if wazevoapi.PrintMachineCodeHexPerFunctionDisassemblable {
		// Inlined data.f32 cannot be disassembled, so we add a dummy instruction here.
		c.Emit4Bytes(dummyInstruction)
	} else {
		c.Emit4Bytes(uint32(rawF32)) // data.f32 xxxxxxx
	}
}

// encodeLoadFpuConst64 encodes the following three instructions:
//
//	ldr x8, #8  ;; literal load of data.f64
//	b 12           ;; skip the data
//	data.f64 xxxxxxx
func encodeLoadFpuConst64(c backend.Compiler, rd uint32, rawF64 uint64) {
	c.Emit4Bytes(
		// https://developer.arm.com/documentation/ddi0596/2020-12/SIMD-FP-Instructions/LDR--literal--SIMD-FP---Load-SIMD-FP-Register--PC-relative-literal--?lang=en
		0b1<<30 | 0b111<<26 | (0x8/4)<<5 | rd,
	)
	c.Emit4Bytes(encodeUnconditionalBranch(false, 12)) // b 12
	if wazevoapi.PrintMachineCodeHexPerFunctionDisassemblable {
		// Inlined data.f64 cannot be disassembled, so we add dummy instructions here.
		c.Emit4Bytes(dummyInstruction)
		c.Emit4Bytes(dummyInstruction)
	} else {
		// data.f64 xxxxxxx
		c.Emit4Bytes(uint32(rawF64))
		c.Emit4Bytes(uint32(rawF64 >> 32))
	}
}

// encodeAluRRRR encodes as Data-processing (3 source) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en
func encodeAluRRRR(op aluOp, rd, rn, rm, ra, _64bit uint32) uint32 {
	var oO, op31 uint32
	switch op {
	case aluOpMAdd:
		op31, oO = 0b000, 0b0
	case aluOpMSub:
		op31, oO = 0b000, 0b1
	default:
		panic("TODO/BUG")
	}
	return _64bit<<31 | 0b11011<<24 | op31<<21 | rm<<16 | oO<<15 | ra<<10 | rn<<5 | rd
}

// encodeBitRR encodes as Data-processing (1 source) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en
func encodeBitRR(op bitOp, rd, rn, _64bit uint32) uint32 {
	var opcode2, opcode uint32
	switch op {
	case bitOpRbit:
		opcode2, opcode = 0b00000, 0b000000
	case bitOpClz:
		opcode2, opcode = 0b00000, 0b000100
	default:
		panic("TODO/BUG")
	}
	return _64bit<<31 | 0b1_0_11010110<<21 | opcode2<<15 | opcode<<10 | rn<<5 | rd
}

func encodeAsMov32(rn, rd uint32) uint32 {
	// This is an alias of ORR (shifted register):
	// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/MOV--register---Move--register---an-alias-of-ORR--shifted-register--
	return encodeLogicalShiftedRegister(0b001, 0, rn, 0, regNumberInEncoding[xzr], rd)
}

// encodeExtend encodes extension instructions.
func encodeExtend(signed bool, from, to byte, rd, rn uint32) uint32 {
	// UTXB: https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/UXTB--Unsigned-Extend-Byte--an-alias-of-UBFM-?lang=en
	// UTXH: https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/UXTH--Unsigned-Extend-Halfword--an-alias-of-UBFM-?lang=en
	// STXB: https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/SXTB--Signed-Extend-Byte--an-alias-of-SBFM-
	// STXH: https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/SXTH--Sign-Extend-Halfword--an-alias-of-SBFM-
	// STXW: https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/SXTW--Sign-Extend-Word--an-alias-of-SBFM-
	var _31to10 uint32
	switch {
	case !signed && from == 8 && to == 32:
		// 32-bit UXTB
		_31to10 = 0b0101001100000000000111
	case !signed && from == 16 && to == 32:
		// 32-bit UXTH
		_31to10 = 0b0101001100000000001111
	case !signed && from == 8 && to == 64:
		// 64-bit UXTB
		_31to10 = 0b0101001100000000000111
	case !signed && from == 16 && to == 64:
		// 64-bit UXTH
		_31to10 = 0b0101001100000000001111
	case !signed && from == 32 && to == 64:
		return encodeAsMov32(rn, rd)
	case signed && from == 8 && to == 32:
		// 32-bit SXTB
		_31to10 = 0b0001001100000000000111
	case signed && from == 16 && to == 32:
		// 32-bit SXTH
		_31to10 = 0b0001001100000000001111
	case signed && from == 8 && to == 64:
		// 64-bit SXTB
		_31to10 = 0b1001001101000000000111
	case signed && from == 16 && to == 64:
		// 64-bit SXTH
		_31to10 = 0b1001001101000000001111
	case signed && from == 32 && to == 64:
		// SXTW
		_31to10 = 0b1001001101000000011111
	default:
		panic("BUG")
	}
	return _31to10<<10 | rn<<5 | rd
}

func encodeLoadOrStore(kind instructionKind, rt uint32, amode addressMode) uint32 {
	var _22to31 uint32
	var bits int64
	switch kind {
	case uLoad8:
		_22to31 = 0b0011100001
		bits = 8
	case sLoad8:
		_22to31 = 0b0011100010
		bits = 8
	case uLoad16:
		_22to31 = 0b0111100001
		bits = 16
	case sLoad16:
		_22to31 = 0b0111100010
		bits = 16
	case uLoad32:
		_22to31 = 0b1011100001
		bits = 32
	case sLoad32:
		_22to31 = 0b1011100010
		bits = 32
	case uLoad64:
		_22to31 = 0b1111100001
		bits = 64
	case fpuLoad32:
		_22to31 = 0b1011110001
		bits = 32
	case fpuLoad64:
		_22to31 = 0b1111110001
		bits = 64
	case fpuLoad128:
		_22to31 = 0b0011110011
		bits = 128
	case store8:
		_22to31 = 0b0011100000
		bits = 8
	case store16:
		_22to31 = 0b0111100000
		bits = 16
	case store32:
		_22to31 = 0b1011100000
		bits = 32
	case store64:
		_22to31 = 0b1111100000
		bits = 64
	case fpuStore32:
		_22to31 = 0b1011110000
		bits = 32
	case fpuStore64:
		_22to31 = 0b1111110000
		bits = 64
	case fpuStore128:
		_22to31 = 0b0011110010
		bits = 128
	default:
		panic("BUG")
	}

	switch amode.kind {
	case addressModeKindRegScaledExtended:
		return encodeLoadOrStoreExtended(_22to31,
			regNumberInEncoding[amode.rn.RealReg()],
			regNumberInEncoding[amode.rm.RealReg()],
			rt, true, amode.extOp)
	case addressModeKindRegScaled:
		return encodeLoadOrStoreExtended(_22to31,
			regNumberInEncoding[amode.rn.RealReg()], regNumberInEncoding[amode.rm.RealReg()],
			rt, true, extendOpNone)
	case addressModeKindRegExtended:
		return encodeLoadOrStoreExtended(_22to31,
			regNumberInEncoding[amode.rn.RealReg()], regNumberInEncoding[amode.rm.RealReg()],
			rt, false, amode.extOp)
	case addressModeKindRegReg:
		return encodeLoadOrStoreExtended(_22to31,
			regNumberInEncoding[amode.rn.RealReg()], regNumberInEncoding[amode.rm.RealReg()],
			rt, false, extendOpNone)
	case addressModeKindRegSignedImm9:
		// e.g. https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/LDUR--Load-Register--unscaled--
		return encodeLoadOrStoreSIMM9(_22to31, 0b00 /* unscaled */, regNumberInEncoding[amode.rn.RealReg()], rt, amode.imm)
	case addressModeKindPostIndex:
		return encodeLoadOrStoreSIMM9(_22to31, 0b01 /* post index */, regNumberInEncoding[amode.rn.RealReg()], rt, amode.imm)
	case addressModeKindPreIndex:
		return encodeLoadOrStoreSIMM9(_22to31, 0b11 /* pre index */, regNumberInEncoding[amode.rn.RealReg()], rt, amode.imm)
	case addressModeKindRegUnsignedImm12:
		// "unsigned immediate" in https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Loads-and-Stores?lang=en
		rn := regNumberInEncoding[amode.rn.RealReg()]
		imm := amode.imm
		div := bits / 8
		if imm != 0 && !offsetFitsInAddressModeKindRegUnsignedImm12(byte(bits), imm) {
			panic("BUG")
		}
		imm /= div
		return _22to31<<22 | 0b1<<24 | uint32(imm&0b111111111111)<<10 | rn<<5 | rt
	default:
		panic("BUG")
	}
}

// encodeAluBitmaskImmediate encodes as Logical (immediate) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Immediate?lang=en
func encodeAluBitmaskImmediate(op aluOp, rd, rn uint32, imm uint64, _64bit bool) uint32 {
	var _31to23 uint32
	switch op {
	case aluOpAnd:
		_31to23 = 0b00_100100
	case aluOpOrr:
		_31to23 = 0b01_100100
	case aluOpEor:
		_31to23 = 0b10_100100
	default:
		panic("BUG")
	}
	if _64bit {
		_31to23 |= 0b1 << 8
	}
	immr, imms, N := bitmaskImmediate(imm, _64bit)
	return _31to23<<23 | uint32(N)<<22 | uint32(immr)<<16 | uint32(imms)<<10 | rn<<5 | rd
}

func bitmaskImmediate(c uint64, is64bit bool) (immr, imms, N byte) {
	var size uint32
	switch {
	case c != c>>32|c<<32:
		size = 64
	case c != c>>16|c<<48:
		size = 32
		c = uint64(int32(c))
	case c != c>>8|c<<56:
		size = 16
		c = uint64(int16(c))
	case c != c>>4|c<<60:
		size = 8
		c = uint64(int8(c))
	case c != c>>2|c<<62:
		size = 4
		c = uint64(int64(c<<60) >> 60)
	default:
		size = 2
		c = uint64(int64(c<<62) >> 62)
	}

	neg := false
	if int64(c) < 0 {
		c = ^c
		neg = true
	}

	onesSize, nonZeroPos := getOnesSequenceSize(c)
	if neg {
		nonZeroPos = onesSize + nonZeroPos
		onesSize = size - onesSize
	}

	var mode byte = 32
	if is64bit && size == 64 {
		N, mode = 0b1, 64
	}

	immr = byte((size - nonZeroPos) & (size - 1) & uint32(mode-1))
	imms = byte((onesSize - 1) | 63&^(size<<1-1))
	return
}

func getOnesSequenceSize(x uint64) (size, nonZeroPos uint32) {
	// Take 0b00111000 for example:
	y := getLowestBit(x)               // = 0b0000100
	nonZeroPos = setBitPos(y)          // = 2
	size = setBitPos(x+y) - nonZeroPos // = setBitPos(0b0100000) - 2 = 5 - 2 = 3
	return
}

func setBitPos(x uint64) (ret uint32) {
	for ; ; ret++ {
		if x == 0b1 {
			break
		}
		x = x >> 1
	}
	return
}

// encodeLoadOrStoreExtended encodes store/load instruction as "extended register offset" in Load/store register (register offset):
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Loads-and-Stores?lang=en
func encodeLoadOrStoreExtended(_22to32 uint32, rn, rm, rt uint32, scaled bool, extOp extendOp) uint32 {
	var option uint32
	switch extOp {
	case extendOpUXTW:
		option = 0b010
	case extendOpSXTW:
		option = 0b110
	case extendOpNone:
		option = 0b111
	default:
		panic("BUG")
	}
	var s uint32
	if scaled {
		s = 0b1
	}
	return _22to32<<22 | 0b1<<21 | rm<<16 | option<<13 | s<<12 | 0b10<<10 | rn<<5 | rt
}

// encodeLoadOrStoreSIMM9 encodes store/load instruction as one of post-index, pre-index or unscaled immediate as in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Loads-and-Stores?lang=en
func encodeLoadOrStoreSIMM9(_22to32, _1011 uint32, rn, rt uint32, imm9 int64) uint32 {
	return _22to32<<22 | (uint32(imm9)&0b111111111)<<12 | _1011<<10 | rn<<5 | rt
}

// encodeFpuRRR encodes as single or double precision (depending on `_64bit`) of Floating-point data-processing (2 source) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeFpuRRR(op fpuBinOp, rd, rn, rm uint32, _64bit bool) (ret uint32) {
	// https://developer.arm.com/documentation/ddi0596/2021-12/SIMD-FP-Instructions/ADD--vector--Add-vectors--scalar--floating-point-and-integer-
	var opcode uint32
	switch op {
	case fpuBinOpAdd:
		opcode = 0b0010
	case fpuBinOpSub:
		opcode = 0b0011
	case fpuBinOpMul:
		opcode = 0b0000
	case fpuBinOpDiv:
		opcode = 0b0001
	case fpuBinOpMax:
		opcode = 0b0100
	case fpuBinOpMin:
		opcode = 0b0101
	default:
		panic("BUG")
	}
	var ptype uint32
	if _64bit {
		ptype = 0b01
	}
	return 0b1111<<25 | ptype<<22 | 0b1<<21 | rm<<16 | opcode<<12 | 0b1<<11 | rn<<5 | rd
}

// encodeAluRRImm12 encodes as Add/subtract (immediate) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Immediate?lang=en
func encodeAluRRImm12(op aluOp, rd, rn uint32, imm12 uint16, shiftBit byte, _64bit bool) uint32 {
	var _31to24 uint32
	switch op {
	case aluOpAdd:
		_31to24 = 0b00_10001
	case aluOpAddS:
		_31to24 = 0b01_10001
	case aluOpSub:
		_31to24 = 0b10_10001
	case aluOpSubS:
		_31to24 = 0b11_10001
	default:
		panic("BUG")
	}
	if _64bit {
		_31to24 |= 0b1 << 7
	}
	return _31to24<<24 | uint32(shiftBit)<<22 | uint32(imm12&0b111111111111)<<10 | rn<<5 | rd
}

// encodeAluRRR encodes as Data Processing (shifted register), depending on aluOp.
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en#addsub_shift
func encodeAluRRRShift(op aluOp, rd, rn, rm, amount uint32, shiftOp shiftOp, _64bit bool) uint32 {
	var _31to24 uint32
	var opc, n uint32
	switch op {
	case aluOpAdd:
		_31to24 = 0b00001011
	case aluOpAddS:
		_31to24 = 0b00101011
	case aluOpSub:
		_31to24 = 0b01001011
	case aluOpSubS:
		_31to24 = 0b01101011
	case aluOpAnd, aluOpOrr, aluOpEor:
		// "Logical (shifted register)".
		switch op {
		case aluOpAnd:
			// all zeros
		case aluOpOrr:
			opc = 0b01
		case aluOpEor:
			opc = 0b10
		}
		_31to24 = 0b000_01010
	default:
		panic(op.String())
	}

	if _64bit {
		_31to24 |= 0b1 << 7
	}

	var shift uint32
	switch shiftOp {
	case shiftOpLSL:
		shift = 0b00
	case shiftOpLSR:
		shift = 0b01
	case shiftOpASR:
		shift = 0b10
	default:
		panic(shiftOp.String())
	}
	return opc<<29 | n<<21 | _31to24<<24 | shift<<22 | rm<<16 | (amount << 10) | (rn << 5) | rd
}

// encodeAluRRR encodes as Data Processing (register), depending on aluOp.
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en
func encodeAluRRR(op aluOp, rd, rn, rm uint32, _64bit, isRnSp bool) uint32 {
	var _31to21, _15to10 uint32
	switch op {
	case aluOpAdd:
		if isRnSp {
			// "Extended register" with UXTW.
			_31to21 = 0b00001011_001
			_15to10 = 0b011000
		} else {
			// "Shifted register" with shift = 0
			_31to21 = 0b00001011_000
		}
	case aluOpAddS:
		if isRnSp {
			panic("TODO")
		}
		// "Shifted register" with shift = 0
		_31to21 = 0b00101011_000
	case aluOpSub:
		if isRnSp {
			// "Extended register" with UXTW.
			_31to21 = 0b01001011_001
			_15to10 = 0b011000
		} else {
			// "Shifted register" with shift = 0
			_31to21 = 0b01001011_000
		}
	case aluOpSubS:
		if isRnSp {
			panic("TODO")
		}
		// "Shifted register" with shift = 0
		_31to21 = 0b01101011_000
	case aluOpAnd, aluOpOrr, aluOpEor:
		// "Logical (shifted register)".
		var opc, n uint32
		switch op {
		case aluOpAnd:
			// all zeros
		case aluOpOrr:
			opc = 0b01
		case aluOpEor:
			opc = 0b10
		}
		_31to21 = 0b000_01010_000 | opc<<8 | n
	case aluOpLsl, aluOpAsr, aluOpLsr, aluOpRotR:
		// "Data-processing (2 source)".
		_31to21 = 0b00011010_110
		switch op {
		case aluOpLsl:
			_15to10 = 0b001000
		case aluOpLsr:
			_15to10 = 0b001001
		case aluOpAsr:
			_15to10 = 0b001010
		case aluOpRotR:
			_15to10 = 0b001011
		}
	case aluOpSDiv:
		// "Data-processing (2 source)".
		_31to21 = 0b11010110
		_15to10 = 0b000011
	case aluOpUDiv:
		// "Data-processing (2 source)".
		_31to21 = 0b11010110
		_15to10 = 0b000010
	default:
		panic(op.String())
	}
	if _64bit {
		_31to21 |= 0b1 << 10
	}
	return _31to21<<21 | rm<<16 | (_15to10 << 10) | (rn << 5) | rd
}

// encodeLogicalShiftedRegister encodes as Logical (shifted register) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Register?lang=en
func encodeLogicalShiftedRegister(sf_opc uint32, shift_N uint32, rm uint32, imm6 uint32, rn, rd uint32) (ret uint32) {
	ret = sf_opc << 29
	ret |= 0b01010 << 24
	ret |= shift_N << 21
	ret |= rm << 16
	ret |= imm6 << 10
	ret |= rn << 5
	ret |= rd
	return
}

// encodeAddSubtractImmediate encodes as Add/subtract (immediate) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Immediate?lang=en
func encodeAddSubtractImmediate(sf_op_s uint32, sh uint32, imm12 uint32, rn, rd uint32) (ret uint32) {
	ret = sf_op_s << 29
	ret |= 0b100010 << 23
	ret |= sh << 22
	ret |= imm12 << 10
	ret |= rn << 5
	ret |= rd
	return
}

// encodePreOrPostIndexLoadStorePair64 encodes as Load/store pair (pre/post-indexed) in
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/LDP--Load-Pair-of-Registers-
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/STP--Store-Pair-of-Registers-
func encodePreOrPostIndexLoadStorePair64(pre bool, load bool, rn, rt, rt2 uint32, imm7 int64) (ret uint32) {
	if imm7%8 != 0 {
		panic("imm7 for pair load/store must be a multiple of 8")
	}
	imm7 /= 8
	ret = rt
	ret |= rn << 5
	ret |= rt2 << 10
	ret |= (uint32(imm7) & 0b1111111) << 15
	if load {
		ret |= 0b1 << 22
	}
	ret |= 0b101010001 << 23
	if pre {
		ret |= 0b1 << 24
	}
	return
}

// encodeUnconditionalBranch encodes as B or BL instructions:
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/B--Branch-
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/BL--Branch-with-Link-
func encodeUnconditionalBranch(link bool, imm26 int64) (ret uint32) {
	if imm26%4 != 0 {
		panic("imm26 for branch must be a multiple of 4")
	}
	imm26 /= 4
	ret = uint32(imm26 & 0b11_11111111_11111111_11111111)
	ret |= 0b101 << 26
	if link {
		ret |= 0b1 << 31
	}
	return
}

// encodeCBZCBNZ encodes as either CBZ or CBNZ:
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/CBZ--Compare-and-Branch-on-Zero-
// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/CBNZ--Compare-and-Branch-on-Nonzero-
func encodeCBZCBNZ(rt uint32, nz bool, imm19 uint32, _64bit bool) (ret uint32) {
	ret = rt
	ret |= imm19 << 5
	if nz {
		ret |= 1 << 24
	}
	ret |= 0b11010 << 25
	if _64bit {
		ret |= 1 << 31
	}
	return
}

// encodeMoveWideImmediate encodes as either MOVZ, MOVN or MOVK, as Move wide (immediate) in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Immediate?lang=en
//
// "shift" must have been divided by 16 at this point.
func encodeMoveWideImmediate(opc uint32, rd uint32, imm, shift, _64bit uint64) (ret uint32) {
	ret = rd
	ret |= uint32(imm&0xffff) << 5
	ret |= (uint32(shift)) << 21
	ret |= 0b100101 << 23
	ret |= opc << 29
	ret |= uint32(_64bit) << 31
	return
}

// encodeAluRRImm encodes as "Bitfield" in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Immediate?lang=en#log_imm
func encodeAluRRImm(op aluOp, rd, rn, amount, _64bit uint32) uint32 {
	var opc uint32
	var immr, imms uint32
	switch op {
	case aluOpLsl:
		// LSL (immediate) is an alias for UBFM.
		// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/UBFM--Unsigned-Bitfield-Move-?lang=en
		opc = 0b10
		if _64bit == 1 {
			immr = 64 - amount
		} else {
			immr = 32 - amount
		}
		imms = immr - 1
	case aluOpLsr:
		// LSR (immediate) is an alias for UBFM.
		// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/LSR--immediate---Logical-Shift-Right--immediate---an-alias-of-UBFM-?lang=en
		opc = 0b10
		imms, immr = 0b011111|_64bit<<5, amount
	case aluOpAsr:
		// ASR (immediate) is an alias for SBFM.
		// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/SBFM--Signed-Bitfield-Move-?lang=en
		opc = 0b00
		imms, immr = 0b011111|_64bit<<5, amount
	default:
		panic(op.String())
	}
	return _64bit<<31 | opc<<29 | 0b100110<<23 | _64bit<<22 | immr<<16 | imms<<10 | rn<<5 | rd
}

// encodeVecLanes encodes as Data Processing (Advanced SIMD across lanes) depending on vecOp in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en
func encodeVecLanes(op vecOp, rd uint32, rn uint32, arr vecArrangement) uint32 {
	var u, q, size, opcode uint32
	switch op {
	case vecOpUaddlv:
		u, opcode = 1, 0b00011
		switch arr {
		case vecArrangement8B:
			q, size = 0b0, 0b00
		case vecArrangement16B:
			q, size = 0b1, 0b00
		case vecArrangement4H:
			q, size = 0, 0b01
		case vecArrangement8H:
			q, size = 1, 0b01
		case vecArrangement4S:
			q, size = 1, 0b10
		default:
			panic("unsupported arrangement: " + arr.String())
		}
	default:
		panic("unsupported or illegal vecOp: " + op.String())
	}
	return q<<30 | u<<29 | 0b1110<<24 | size<<22 | 0b11000<<17 | opcode<<12 | 0b10<<10 | rn<<5 | rd
}

// encodeVecMisc encodes as Data Processing (Advanced SIMD two-register miscellaneous) depending on vecOp in
// https://developer.arm.com/documentation/ddi0596/2020-12/Index-by-Encoding/Data-Processing----Scalar-Floating-Point-and-Advanced-SIMD?lang=en#simd-dp
func encodeAdvancedSIMDTwoMisc(op vecOp, rd, rn uint32, arr vecArrangement) uint32 {
	var q, u, size, opcode uint32
	switch op {
	case vecOpCnt:
		opcode = 0b00101
		switch arr {
		case vecArrangement8B:
			q, size = 0b0, 0b00
		case vecArrangement16B:
			q, size = 0b1, 0b00
		default:
			panic("unsupported arrangement: " + arr.String())
		}
	default:
		panic("unsupported or illegal vecOp: " + op.String())
	}
	return q<<30 | u<<29 | 0b01110<<24 | size<<22 | 0b10000<<17 | opcode<<12 | 0b10<<10 | rn<<5 | rd
}

// brTableSequenceOffsetTableBegin is the offset inside the brTableSequence where the table begins after 4 instructions
const brTableSequenceOffsetTableBegin = 16

func encodeBrTableSequence(c backend.Compiler, index regalloc.VReg, targets []uint32) {
	tmpRegNumber := regNumberInEncoding[tmp]
	indexNumber := regNumberInEncoding[index.RealReg()]

	// adr tmpReg, PC+16 (PC+16 is the address of the first label offset)
	// ldrsw index, [tmpReg, index, UXTW 2] ;; index = int64(*(tmpReg + index*8))
	// add tmpReg, tmpReg, index
	// br tmpReg
	// [offset_to_l1, offset_to_l2, ..., offset_to_lN]
	c.Emit4Bytes(encodeAdr(tmpRegNumber, 16))
	c.Emit4Bytes(encodeLoadOrStore(sLoad32, indexNumber,
		addressMode{kind: addressModeKindRegScaledExtended, rn: tmpRegVReg, rm: index, extOp: extendOpUXTW},
	))
	c.Emit4Bytes(encodeAluRRR(aluOpAdd, tmpRegNumber, tmpRegNumber, indexNumber, true, false))
	c.Emit4Bytes(encodeUnconditionalBranchReg(tmpRegNumber, false))

	// Offsets are resolved in ResolveRelativeAddress phase.
	for _, offset := range targets {
		if wazevoapi.PrintMachineCodeHexPerFunctionDisassemblable {
			// Inlined offset tables cannot be disassembled properly, so pad dummy instructions to make the debugging easier.
			c.Emit4Bytes(dummyInstruction)
		} else {
			c.Emit4Bytes(offset)
		}
	}
}

// encodeExitSequence matches the implementation detail of abiImpl.emitGoEntryPreamble.
func encodeExitSequence(c backend.Compiler, ctxReg regalloc.VReg) {
	// Restore the FP, SP and LR, and return to the Go code:
	// 		ldr fp, [savedExecutionContextPtr, #OriginalFramePointer]
	// 		ldr tmp, [savedExecutionContextPtr, #OriginalStackPointer]
	//      mov sp, tmp ;; sp cannot be str'ed directly.
	// 		ldr lr, [savedExecutionContextPtr, #GoReturnAddress]
	// 		ret ;; --> return to the Go code

	restoreFp := encodeLoadOrStore(
		uLoad64,
		regNumberInEncoding[fp],
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   ctxReg,
			imm:  wazevoapi.ExecutionContextOffsets.OriginalFramePointer.I64(),
		},
	)

	restoreSpToTmp := encodeLoadOrStore(
		uLoad64,
		regNumberInEncoding[tmp],
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   ctxReg,
			imm:  wazevoapi.ExecutionContextOffsets.OriginalStackPointer.I64(),
		},
	)

	movTmpToSp := encodeAddSubtractImmediate(0b100, 0, 0,
		regNumberInEncoding[tmp], regNumberInEncoding[sp])

	restoreLr := encodeLoadOrStore(
		uLoad64,
		regNumberInEncoding[lr],
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   ctxReg,
			imm:  wazevoapi.ExecutionContextOffsets.GoReturnAddress.I64(),
		},
	)

	c.Emit4Bytes(restoreFp)
	c.Emit4Bytes(restoreSpToTmp)
	c.Emit4Bytes(movTmpToSp)
	c.Emit4Bytes(restoreLr)
	c.Emit4Bytes(encodeRet())
}

func encodeRet() uint32 {
	// https://developer.arm.com/documentation/ddi0596/2020-12/Base-Instructions/RET--Return-from-subroutine-?lang=en
	return 0b1101011001011111<<16 | regNumberInEncoding[lr]<<5
}
