package arm64

// Files prefixed as lower_instr** do the instruction selection, meaning that lowering SSA level instructions
// into machine specific instructions.
//
// Importantly, what the lower** functions does includes tree-matching; find the pattern from the given instruction tree,
// and merge the multiple instructions if possible. It can be considered as "N:1" instruction selection.

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
)

// LowerSingleBranch implements backend.Machine.
func (m *machine) LowerSingleBranch(br *ssa.Instruction) {
	switch br.Opcode() {
	case ssa.OpcodeJump:
		_, _, targetBlk := br.BranchData()
		if br.IsFallthroughJump() {
			return
		}
		b := m.allocateInstr()
		target := m.getOrAllocateSSABlockLabel(targetBlk)
		if target == returnLabel {
			b.asRet(m.currentABI)
		} else {
			b.asBr(target)
		}
		m.insert(b)
	case ssa.OpcodeBrTable:
		m.lowerBrTable(br)
	default:
		panic("BUG: unexpected branch opcode" + br.Opcode().String())
	}
}

func (m *machine) lowerBrTable(i *ssa.Instruction) {
	index, targets := i.BrTableData()
	indexOperand := m.getOperand_NR(m.compiler.ValueDefinition(index), extModeNone)

	// Firstly, we have to do the bounds check of the index, and
	// set it to the default target (sitting at the end of the list) if it's out of bounds.

	// mov  maxIndexReg #maximum_index
	// subs wzr, index, maxIndexReg
	// csel adjustedIndex, maxIndexReg, index, hs ;; if index is higher or equal than maxIndexReg.
	maxIndexReg := m.compiler.AllocateVReg(regalloc.RegTypeInt)
	m.lowerConstantI32(maxIndexReg, int32(len(targets)-1))
	subs := m.allocateInstr()
	subs.asALU(aluOpSubS, operandNR(xzrVReg), indexOperand, operandNR(maxIndexReg), false)
	m.insert(subs)
	csel := m.allocateInstr()
	adjustedIndex := m.compiler.AllocateVReg(regalloc.RegTypeInt)
	csel.asCSel(operandNR(adjustedIndex), operandNR(maxIndexReg), indexOperand, hs, false)
	m.insert(csel)

	brSequence := m.allocateInstr()

	// TODO: reuse the slice!
	labels := make([]uint32, len(targets))
	for j, target := range targets {
		labels[j] = uint32(m.getOrAllocateSSABlockLabel(target))
	}

	brSequence.asBrTableSequence(adjustedIndex, labels)
	m.insert(brSequence)
}

// LowerConditionalBranch implements backend.Machine.
func (m *machine) LowerConditionalBranch(b *ssa.Instruction) {
	cval, args, targetBlk := b.BranchData()
	if len(args) > 0 {
		panic(fmt.Sprintf(
			"conditional branch shouldn't have args; likely a bug in critical edge splitting: from %s to %s",
			m.currentSSABlk,
			targetBlk,
		))
	}

	target := m.getOrAllocateSSABlockLabel(targetBlk)
	cvalDef := m.compiler.ValueDefinition(cval)

	switch {
	case m.compiler.MatchInstr(cvalDef, ssa.OpcodeIcmp): // This case, we can use the ALU flag set by SUBS instruction.
		cvalInstr := cvalDef.Instr
		x, y, c := cvalInstr.IcmpData()
		cc, signed := condFlagFromSSAIntegerCmpCond(c), c.Signed()
		if b.Opcode() == ssa.OpcodeBrz {
			cc = cc.invert()
		}

		m.lowerIcmpToFlag(x, y, signed)
		cbr := m.allocateInstr()
		cbr.asCondBr(cc.asCond(), target, false /* ignored */)
		m.insert(cbr)
		m.compiler.MarkLowered(cvalDef.Instr)
	case m.compiler.MatchInstr(cvalDef, ssa.OpcodeFcmp): // This case we can use the Fpu flag directly.
		cvalInstr := cvalDef.Instr
		x, y, c := cvalInstr.FcmpData()
		cc := condFlagFromSSAFloatCmpCond(c)
		if b.Opcode() == ssa.OpcodeBrz {
			cc = cc.invert()
		}
		m.lowerFcmpToFlag(x, y)
		cbr := m.allocateInstr()
		cbr.asCondBr(cc.asCond(), target, false /* ignored */)
		m.insert(cbr)
		m.compiler.MarkLowered(cvalDef.Instr)
	default:
		rn := m.getOperand_NR(cvalDef, extModeNone)
		var c cond
		if b.Opcode() == ssa.OpcodeBrz {
			c = registerAsRegZeroCond(rn.nr())
		} else {
			c = registerAsRegNotZeroCond(rn.nr())
		}
		cbr := m.allocateInstr()
		cbr.asCondBr(c, target, false)
		m.insert(cbr)
	}
}

// LowerInstr implements backend.Machine.
func (m *machine) LowerInstr(instr *ssa.Instruction) {
	switch op := instr.Opcode(); op {
	case ssa.OpcodeBrz, ssa.OpcodeBrnz, ssa.OpcodeJump, ssa.OpcodeBrTable:
		panic("BUG: branching instructions are handled by LowerBranches")
	case ssa.OpcodeReturn:
		panic("BUG: return must be handled by backend.Compiler")
	case ssa.OpcodeIadd, ssa.OpcodeIsub:
		m.lowerSubOrAdd(instr, op == ssa.OpcodeIadd)
	case ssa.OpcodeFadd, ssa.OpcodeFsub, ssa.OpcodeFmul, ssa.OpcodeFdiv, ssa.OpcodeFmax, ssa.OpcodeFmin:
		m.lowerFpuBinOp(instr)
	case ssa.OpcodeIconst, ssa.OpcodeF32const, ssa.OpcodeF64const: // Constant instructions are inlined.
	case ssa.OpcodeExitWithCode:
		execCtx, code := instr.ExitWithCodeData()
		m.lowerExitWithCode(m.compiler.VRegOf(execCtx), code)
	case ssa.OpcodeExitIfTrueWithCode:
		execCtx, c, code := instr.ExitIfTrueWithCodeData()
		m.lowerExitIfTrueWithCode(m.compiler.VRegOf(execCtx), c, code)
	case ssa.OpcodeStore, ssa.OpcodeIstore8, ssa.OpcodeIstore16, ssa.OpcodeIstore32:
		m.lowerStore(instr)
	case ssa.OpcodeLoad:
		m.lowerLoad(instr)
	case ssa.OpcodeUload8, ssa.OpcodeUload16, ssa.OpcodeUload32, ssa.OpcodeSload8, ssa.OpcodeSload16, ssa.OpcodeSload32:
		m.lowerExtLoad(instr)
	case ssa.OpcodeCall, ssa.OpcodeCallIndirect:
		m.lowerCall(instr)
	case ssa.OpcodeIcmp:
		m.lowerIcmp(instr)
	case ssa.OpcodeBand:
		m.lowerBitwiseAluOp(instr, aluOpAnd)
	case ssa.OpcodeBor:
		m.lowerBitwiseAluOp(instr, aluOpOrr)
	case ssa.OpcodeBxor:
		m.lowerBitwiseAluOp(instr, aluOpEor)
	case ssa.OpcodeIshl:
		m.lowerShifts(instr, extModeNone, aluOpLsl)
	case ssa.OpcodeSshr:
		if instr.Return().Type().Bits() == 64 {
			m.lowerShifts(instr, extModeSignExtend64, aluOpLsr)
		} else {
			m.lowerShifts(instr, extModeSignExtend32, aluOpLsr)
		}
	case ssa.OpcodeUshr:
		if instr.Return().Type().Bits() == 64 {
			m.lowerShifts(instr, extModeZeroExtend64, aluOpAsr)
		} else {
			m.lowerShifts(instr, extModeZeroExtend32, aluOpAsr)
		}
	case ssa.OpcodeRotl:
		m.lowerRotl(instr)
	case ssa.OpcodeRotr:
		m.lowerRotr(instr)
	case ssa.OpcodeSExtend, ssa.OpcodeUExtend:
		from, to, signed := instr.ExtendData()
		m.lowerExtend(instr.Arg(), instr.Return(), from, to, signed)
	case ssa.OpcodeFcmp:
		x, y, c := instr.FcmpData()
		m.lowerFcmp(x, y, instr.Return(), c)
	case ssa.OpcodeImul:
		x, y := instr.BinaryData()
		result := instr.Return()
		m.lowerImul(x, y, result)
	case ssa.OpcodeUndefined:
		undef := m.allocateInstr()
		undef.asUDF()
		m.insert(undef)
	case ssa.OpcodeSelect:
		c, x, y := instr.SelectData()
		m.lowerSelect(c, x, y, instr.Return())
	case ssa.OpcodeClz:
		x := instr.UnaryData()
		result := instr.Return()
		m.lowerClz(x, result)
	case ssa.OpcodeCtz:
		x := instr.UnaryData()
		result := instr.Return()
		m.lowerCtz(x, result)
	case ssa.OpcodePopcnt:
		x := instr.UnaryData()
		result := instr.Return()
		m.lowerPopcnt(x, result)
	case ssa.OpcodeFcvtFromSint:
		x := instr.UnaryData()
		result := instr.Return()
		m.lowerIntToFpu(result, x, true, x.Type() == ssa.TypeI64, result.Type().Bits() == 64)
	case ssa.OpcodeFcvtFromUint:
		x := instr.UnaryData()
		result := instr.Return()
		m.lowerIntToFpu(result, x, false, x.Type() == ssa.TypeI64, result.Type().Bits() == 64)
	case ssa.OpcodeFpromote:
		v := instr.UnaryData()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(v), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		cnt := m.allocateInstr()
		cnt.asVecMisc(vecOpCvt32To64, rd, rn, vecArrangementNone)
		m.insert(cnt)
	case ssa.OpcodeIreduce:
		rn := m.getOperand_NR(m.compiler.ValueDefinition(instr.UnaryData()), extModeNone)
		retVal := instr.Return()
		rd := m.compiler.VRegOf(retVal)

		if retVal.Type() != ssa.TypeI32 {
			panic("TODO?: Ireduce to non-i32")
		}
		mov := m.allocateInstr()
		mov.asMove32(rd, rn.reg())
		m.insert(mov)
	default:
		panic("TODO: lowering " + instr.Opcode().String())
	}
	m.FlushPendingInstructions()
}

func (m *machine) lowerIntToFpu(dst, src ssa.Value, signed, src64bit, dst64bit bool) {
	rn := m.getOperand_NR(m.compiler.ValueDefinition(src), extModeNone)
	rd := operandNR(m.compiler.VRegOf(dst))
	cvt := m.allocateInstr()
	cvt.asIntToFpu(rd, rn, signed, src64bit, dst64bit)
	m.insert(cvt)
}

func (m *machine) lowerFpuBinOp(si *ssa.Instruction) {
	instr := m.allocateInstr()
	var op fpuBinOp
	switch si.Opcode() {
	case ssa.OpcodeFadd:
		op = fpuBinOpAdd
	case ssa.OpcodeFsub:
		op = fpuBinOpSub
	case ssa.OpcodeFmul:
		op = fpuBinOpMul
	case ssa.OpcodeFdiv:
		op = fpuBinOpDiv
	case ssa.OpcodeFmax:
		op = fpuBinOpMax
	case ssa.OpcodeFmin:
		op = fpuBinOpMin
	}
	x, y := si.BinaryData()
	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm := m.getOperand_NR(yDef, extModeNone)
	rd := operandNR(m.compiler.VRegOf(si.Return()))
	instr.asFpuRRR(op, rd, rn, rm, x.Type().Bits() == 64)
	m.insert(instr)
}

func (m *machine) lowerSubOrAdd(si *ssa.Instruction, add bool) {
	x, y := si.BinaryData()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm, yNegated := m.getOperand_MaybeNegatedImm12_ER_SR_NR(yDef, extModeNone)

	var aop aluOp
	switch {
	case add && !yNegated: // rn+rm = x+y
		aop = aluOpAdd
	case add && yNegated: // rn-rm = x-(-y) = x+y
		aop = aluOpSub
	case !add && !yNegated: // rn-rm = x-y
		aop = aluOpSub
	case !add && yNegated: // rn+rm = x-(-y) = x-y
		aop = aluOpAdd
	}
	rd := operandNR(m.compiler.VRegOf(si.Return()))
	alu := m.allocateInstr()
	alu.asALU(aop, rd, rn, rm, x.Type().Bits() == 64)
	m.insert(alu)
}

// InsertMove implements backend.Machine.
func (m *machine) InsertMove(dst, src regalloc.VReg) {
	instr := m.allocateInstr()
	switch src.RegType() {
	case regalloc.RegTypeInt:
		instr.asMove64(dst, src)
	case regalloc.RegTypeFloat:
		instr.asFpuMov64(dst, src)
	default:
		panic("TODO")
	}
	m.insert(instr)
}

func (m *machine) lowerIcmp(si *ssa.Instruction) {
	x, y, c := si.IcmpData()
	flag := condFlagFromSSAIntegerCmpCond(c)

	in64bit := x.Type().Bits() == 64
	var ext extMode
	if in64bit {
		if c.Signed() {
			ext = extModeSignExtend64
		} else {
			ext = extModeZeroExtend64
		}
	} else {
		if c.Signed() {
			ext = extModeSignExtend32
		} else {
			ext = extModeZeroExtend32
		}
	}

	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), ext)
	rm := m.getOperand_Imm12_ER_SR_NR(m.compiler.ValueDefinition(y), ext)
	alu := m.allocateInstr()
	alu.asALU(aluOpSubS, operandNR(xzrVReg), rn, rm, in64bit)
	m.insert(alu)

	cset := m.allocateInstr()
	cset.asCSet(m.compiler.VRegOf(si.Return()), flag)
	m.insert(cset)
}

func (m *machine) lowerShifts(si *ssa.Instruction, ext extMode, aluOp aluOp) {
	x, amount := si.BinaryData()
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), ext)
	rm := m.getOperand_ShiftImm_NR(m.compiler.ValueDefinition(amount), ext, x.Type().Bits())
	rd := operandNR(m.compiler.VRegOf(si.Return()))

	alu := m.allocateInstr()
	alu.asALUShift(aluOp, rd, rn, rm, x.Type().Bits() == 64)
	m.insert(alu)
}

func (m *machine) lowerBitwiseAluOp(si *ssa.Instruction, op aluOp) {
	x, y := si.BinaryData()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm := m.getOperand_SR_NR(yDef, extModeNone)
	rd := operandNR(m.compiler.VRegOf(si.Return()))

	alu := m.allocateInstr()
	alu.asALU(op, rd, rn, rm, si.Return().Type().Bits() == 64)
	m.insert(alu)
}

func (m *machine) lowerRotl(si *ssa.Instruction) {
	x, y := si.BinaryData()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	// Encode rotl as neg + rotr: neg is really a sub against the zero-reg.

	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm := m.getOperand_NR(yDef, extModeNone)
	rd := operandNR(m.compiler.VRegOf(si.Return()))

	// Encode neg as sub $reg, xzr, $reg.
	neg := m.allocateInstr()
	neg.asALU(aluOpSub, rn, operandNR(xzrVReg), rn, si.Return().Type().Bits() == 64)
	m.insert(neg)

	alu := m.allocateInstr()
	alu.asALU(aluOpRotR, rd, rn, rm, si.Return().Type().Bits() == 64)
	m.insert(alu)
}

func (m *machine) lowerRotr(si *ssa.Instruction) {
	x, y := si.BinaryData()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm := m.getOperand_NR(yDef, extModeNone)
	rd := operandNR(m.compiler.VRegOf(si.Return()))

	alu := m.allocateInstr()
	alu.asALU(aluOpRotR, rd, rn, rm, si.Return().Type().Bits() == 64)
	m.insert(alu)
}

func (m *machine) lowerExtend(arg, ret ssa.Value, from, to byte, signed bool) {
	rd := m.compiler.VRegOf(ret)
	rn := m.getOperand_NR(m.compiler.ValueDefinition(arg), extModeNone)

	ext := m.allocateInstr()
	ext.asExtend(rd, rn.nr(), from, to, signed)
	m.insert(ext)
}

func (m *machine) lowerFcmp(x, y, result ssa.Value, c ssa.FloatCmpCond) {
	rn, rm := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone), m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)

	fc := m.allocateInstr()
	fc.asFpuCmp(rn, rm, x.Type().Bits() == 64)
	m.insert(fc)

	cset := m.allocateInstr()
	cset.asCSet(m.compiler.VRegOf(result), condFlagFromSSAFloatCmpCond(c))
	m.insert(cset)
}

func (m *machine) lowerImul(x, y, result ssa.Value) {
	rd := m.compiler.VRegOf(result)
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)

	// TODO: if this comes before Add/Sub, we could merge it by putting it into the place of xzrVReg.

	mul := m.allocateInstr()
	mul.asALURRRR(aluOpMAdd, operandNR(rd), rn, rm, operandNR(xzrVReg), x.Type().Bits() == 64)
	m.insert(mul)
}

func (m *machine) lowerClz(x, result ssa.Value) {
	rd := m.compiler.VRegOf(result)
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	clz := m.allocateInstr()
	clz.asBitRR(bitOpClz, rd, rn.nr(), x.Type().Bits() == 64)
	m.insert(clz)
}

func (m *machine) lowerCtz(x, result ssa.Value) {
	rd := m.compiler.VRegOf(result)
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rbit := m.allocateInstr()
	rbit.asBitRR(bitOpRbit, tmpRegVReg, rn.nr(), x.Type().Bits() == 64)
	m.insert(rbit)

	clz := m.allocateInstr()
	clz.asBitRR(bitOpClz, rd, tmpRegVReg, x.Type().Bits() == 64)
	m.insert(clz)
}

func (m *machine) lowerPopcnt(x, result ssa.Value) {
	// arm64 doesn't have an instruction for population count on scalar register,
	// so we use the vector instruction `cnt`.
	// This is exactly what the official Go implements bits.OneCount.
	// For example, "func () int { return bits.OneCount(10) }" is compiled as
	//
	//    MOVD    $10, R0 ;; Load 10.
	//    FMOVD   R0, F0
	//    VCNT    V0.B8, V0.B8
	//    UADDLV  V0.B8, V0
	//
	// In aarch64 asm, FMOVD is encoded as `ins`, VCNT is `cnt`,
	// and the registers may use different names. In our encoding we use the following
	// instructions:
	//
	//    ins v0.d[0], x0     ;; mov from GPR to vec (FMOV above) is encoded as INS
	//    cnt v0.16b, v0.16b  ;; we use vec arrangement 16b
	//    uaddlv h0, v0.8b    ;; h0 is still v0 with the dest width specifier 'H', implied when src arrangement is 8b
	//    mov x5, v0.d[0]     ;; finally we mov the result back to a GPR
	//

	rd := operandNR(m.compiler.VRegOf(result))
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)

	rf1 := operandNR(m.compiler.AllocateVReg(regalloc.RegTypeFloat))
	ins := m.allocateInstr()
	ins.asMovToVec(rf1, rn, vecArrangementD, vecIndex(0))
	m.insert(ins)

	rf2 := operandNR(m.compiler.AllocateVReg(regalloc.RegTypeFloat))
	cnt := m.allocateInstr()
	cnt.asVecMisc(vecOpCnt, rf2, rf1, vecArrangement16B)
	m.insert(cnt)

	rf3 := operandNR(m.compiler.AllocateVReg(regalloc.RegTypeFloat))
	uaddlv := m.allocateInstr()
	uaddlv.asVecLanes(vecOpUaddlv, rf3, rf2, vecArrangement8B)
	m.insert(uaddlv)

	mov := m.allocateInstr()
	mov.asMovFromVec(rd, rf3, vecArrangementD, vecIndex(0))
	m.insert(mov)
}

const exitWithCodeEncodingSize = exitSequenceSize + 8

// lowerExitWithCode lowers the lowerExitWithCode takes a context pointer as argument.
func (m *machine) lowerExitWithCode(execCtxVReg regalloc.VReg, code wazevoapi.ExitCode) {
	loadExitCodeConst := m.allocateInstr()
	loadExitCodeConst.asMOVZ(tmpRegVReg, uint64(code), 0, true)

	setExitCode := m.allocateInstr()
	setExitCode.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtxVReg, imm: wazevoapi.ExecutionContextOffsets.ExitCodeOffset.I64(),
		}, 32)

	exitSeq := m.allocateInstr()
	exitSeq.asExitSequence(execCtxVReg)

	m.insert(loadExitCodeConst)
	m.insert(setExitCode)
	m.insert(exitSeq)
}

func (m *machine) lowerIcmpToFlag(x, y ssa.Value, signed bool) {
	if x.Type() != y.Type() {
		panic("TODO(maybe): support icmp with different types")
	}

	extMod := extModeOf(x.Type(), signed)

	// First operand must be in pure register form.
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extMod)
	// Second operand can be in any of Imm12, ER, SR, or NR form supported by the SUBS instructions.
	rm := m.getOperand_Imm12_ER_SR_NR(m.compiler.ValueDefinition(y), extMod)

	alu := m.allocateInstr()
	// subs zr, rn, rm
	alu.asALU(
		aluOpSubS,
		// We don't need the result, just need to set flags.
		operandNR(xzrVReg),
		rn,
		rm,
		x.Type().Bits() == 64,
	)
	m.insert(alu)
}

func (m *machine) lowerFcmpToFlag(x, y ssa.Value) {
	if x.Type() != y.Type() {
		panic("TODO(maybe): support icmp with different types")
	}

	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
	cmp := m.allocateInstr()
	cmp.asFpuCmp(rn, rm, x.Type().Bits() == 64)
	m.insert(cmp)
}

func (m *machine) lowerExitIfTrueWithCode(execCtxVReg regalloc.VReg, cond ssa.Value, code wazevoapi.ExitCode) {
	condDef := m.compiler.ValueDefinition(cond)
	if !m.compiler.MatchInstr(condDef, ssa.OpcodeIcmp) {
		// We can have general case just like cachine.LowerConditionalBranch.
		panic("TODO: OpcodeExitIfTrueWithCode must come after Icmp at the moment")
	}
	m.compiler.MarkLowered(condDef.Instr)

	cvalInstr := condDef.Instr
	x, y, c := cvalInstr.IcmpData()
	signed := c.Signed()
	m.lowerIcmpToFlag(x, y, signed)

	// We have to skip the entire exit sequence if the condition is false.
	cbr := m.allocateInstr()
	cbr.asCondBr(condFlagFromSSAIntegerCmpCond(c).invert().asCond(), invalidLabel, false /* ignored */)
	cbr.condBrOffsetResolve(exitWithCodeEncodingSize + 4 /* br offset is from the beginning of this instruction */)
	m.insert(cbr)
	m.lowerExitWithCode(execCtxVReg, code)
}

func (m *machine) lowerSelect(c, x, y, result ssa.Value) {
	cvalDef := m.compiler.ValueDefinition(c)

	var cc condFlag
	switch {
	case m.compiler.MatchInstr(cvalDef, ssa.OpcodeIcmp): // This case, we can use the ALU flag set by SUBS instruction.
		cvalInstr := cvalDef.Instr
		x, y, c := cvalInstr.IcmpData()
		cc = condFlagFromSSAIntegerCmpCond(c)
		m.lowerIcmpToFlag(x, y, c.Signed())
		m.compiler.MarkLowered(cvalDef.Instr)
	case m.compiler.MatchInstr(cvalDef, ssa.OpcodeFcmp): // This case we can use the Fpu flag directly.
		cvalInstr := cvalDef.Instr
		x, y, c := cvalInstr.FcmpData()
		cc = condFlagFromSSAFloatCmpCond(c)
		m.lowerFcmpToFlag(x, y)
		m.compiler.MarkLowered(cvalDef.Instr)
	default:
		rn := m.getOperand_NR(cvalDef, extModeNone)
		if c.Type() != ssa.TypeI32 && c.Type() != ssa.TypeI64 {
			panic("TODO?BUG?: support select with non-integer condition")
		}
		alu := m.allocateInstr()
		// subs zr, rn, zr
		alu.asALU(
			aluOpSubS,
			// We don't need the result, just need to set flags.
			operandNR(xzrVReg),
			rn,
			operandNR(xzrVReg),
			c.Type().Bits() == 64,
		)
		m.insert(alu)
		cc = ne
	}

	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)

	rd := operandNR(m.compiler.VRegOf(result))
	switch x.Type() {
	case ssa.TypeI32, ssa.TypeI64:
		// csel rd, rn, rm, cc
		csel := m.allocateInstr()
		csel.asCSel(rd, rn, rm, cc, x.Type().Bits() == 64)
		m.insert(csel)
	case ssa.TypeF32, ssa.TypeF64:
		// fcsel rd, rn, rm, cc
		fcsel := m.allocateInstr()
		fcsel.asFpuCSel(rd, rn, rm, cc, x.Type().Bits() == 64)
		m.insert(fcsel)
	}
}
