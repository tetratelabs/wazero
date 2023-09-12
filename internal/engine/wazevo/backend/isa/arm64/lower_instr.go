package arm64

// Files prefixed as lower_instr** do the instruction selection, meaning that lowering SSA level instructions
// into machine specific instructions.
//
// Importantly, what the lower** functions does includes tree-matching; find the pattern from the given instruction tree,
// and merge the multiple instructions if possible. It can be considered as "N:1" instruction selection.

import (
	"fmt"
	"math"

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
		ptr, offset, _ := instr.LoadData()
		ret := m.compiler.VRegOf(instr.Return())
		m.lowerExtLoad(op, ptr, offset, ret)
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
			m.lowerShifts(instr, extModeSignExtend64, aluOpAsr)
		} else {
			m.lowerShifts(instr, extModeSignExtend32, aluOpAsr)
		}
	case ssa.OpcodeUshr:
		if instr.Return().Type().Bits() == 64 {
			m.lowerShifts(instr, extModeZeroExtend64, aluOpLsr)
		} else {
			m.lowerShifts(instr, extModeZeroExtend32, aluOpLsr)
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
		x, y := instr.Arg2()
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
		x := instr.Arg()
		result := instr.Return()
		m.lowerClz(x, result)
	case ssa.OpcodeCtz:
		x := instr.Arg()
		result := instr.Return()
		m.lowerCtz(x, result)
	case ssa.OpcodePopcnt:
		x := instr.Arg()
		result := instr.Return()
		m.lowerPopcnt(x, result)
	case ssa.OpcodeFcvtToSint, ssa.OpcodeFcvtToSintSat:
		x, ctx := instr.Arg2()
		result := instr.Return()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rd := operandNR(m.compiler.VRegOf(result))
		ctxVReg := m.compiler.VRegOf(ctx)
		m.lowerFpuToInt(rd, rn, ctxVReg, true, x.Type() == ssa.TypeF64,
			result.Type().Bits() == 64, op == ssa.OpcodeFcvtToSintSat)
	case ssa.OpcodeFcvtToUint, ssa.OpcodeFcvtToUintSat:
		x, ctx := instr.Arg2()
		result := instr.Return()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rd := operandNR(m.compiler.VRegOf(result))
		ctxVReg := m.compiler.VRegOf(ctx)
		m.lowerFpuToInt(rd, rn, ctxVReg, false, x.Type() == ssa.TypeF64,
			result.Type().Bits() == 64, op == ssa.OpcodeFcvtToUintSat)
	case ssa.OpcodeFcvtFromSint:
		x := instr.Arg()
		result := instr.Return()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rd := operandNR(m.compiler.VRegOf(result))
		m.lowerIntToFpu(rd, rn, true, x.Type() == ssa.TypeI64, result.Type().Bits() == 64)
	case ssa.OpcodeFcvtFromUint:
		x := instr.Arg()
		result := instr.Return()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rd := operandNR(m.compiler.VRegOf(result))
		m.lowerIntToFpu(rd, rn, false, x.Type() == ssa.TypeI64, result.Type().Bits() == 64)
	case ssa.OpcodeFdemote:
		v := instr.Arg()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(v), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		cnt := m.allocateInstr()
		cnt.asFpuRR(fpuUniOpCvt64To32, rd, rn, false)
		m.insert(cnt)
	case ssa.OpcodeFpromote:
		v := instr.Arg()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(v), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		cnt := m.allocateInstr()
		cnt.asFpuRR(fpuUniOpCvt32To64, rd, rn, true)
		m.insert(cnt)
	case ssa.OpcodeIreduce:
		rn := m.getOperand_NR(m.compiler.ValueDefinition(instr.Arg()), extModeNone)
		retVal := instr.Return()
		rd := m.compiler.VRegOf(retVal)

		if retVal.Type() != ssa.TypeI32 {
			panic("TODO?: Ireduce to non-i32")
		}
		mov := m.allocateInstr()
		mov.asMove32(rd, rn.reg())
		m.insert(mov)
	case ssa.OpcodeFneg:
		m.lowerFpuUniOp(fpuUniOpNeg, instr.Arg(), instr.Return())
	case ssa.OpcodeSqrt:
		m.lowerFpuUniOp(fpuUniOpSqrt, instr.Arg(), instr.Return())
	case ssa.OpcodeCeil:
		m.lowerFpuUniOp(fpuUniOpRoundPlus, instr.Arg(), instr.Return())
	case ssa.OpcodeFloor:
		m.lowerFpuUniOp(fpuUniOpRoundMinus, instr.Arg(), instr.Return())
	case ssa.OpcodeTrunc:
		m.lowerFpuUniOp(fpuUniOpRoundZero, instr.Arg(), instr.Return())
	case ssa.OpcodeNearest:
		m.lowerFpuUniOp(fpuUniOpRoundNearest, instr.Arg(), instr.Return())
	case ssa.OpcodeFabs:
		m.lowerFpuUniOp(fpuUniOpAbs, instr.Arg(), instr.Return())
	case ssa.OpcodeBitcast:
		m.lowerBitcast(instr)
	case ssa.OpcodeFcopysign:
		x, y := instr.Arg2()
		m.lowerFcopysign(x, y, instr.Return())
	case ssa.OpcodeSdiv, ssa.OpcodeUdiv:
		x, y, ctx := instr.Arg3()
		ctxVReg := m.compiler.VRegOf(ctx)
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		m.lowerIDiv(ctxVReg, rd, rn, rm, x.Type() == ssa.TypeI64, op == ssa.OpcodeSdiv)
	case ssa.OpcodeSrem, ssa.OpcodeUrem:
		x, y, ctx := instr.Arg3()
		ctxVReg := m.compiler.VRegOf(ctx)
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		m.lowerIRem(ctxVReg, rd, rn, rm, x.Type() == ssa.TypeI64, op == ssa.OpcodeSrem)
	case ssa.OpcodeVconst:
		result := m.compiler.VRegOf(instr.Return())
		lo, hi := instr.VconstData()
		v := m.allocateInstr()
		v.asLoadFpuConst128(result, lo, hi)
		m.insert(v)
	case ssa.OpcodeVIadd:
		x, y, lane := instr.Arg2WithLane()
		arr := ssaLeneToArrangement(lane)
		add := m.allocateInstr()
		rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
		rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
		rd := operandNR(m.compiler.VRegOf(instr.Return()))
		add.asVecRRR(vecOpAdd, rd, rn, rm, arr)
		m.insert(add)
	default:
		panic("TODO: lowering " + op.String())
	}
	m.FlushPendingInstructions()
}

func (m *machine) lowerIRem(execCtxVReg regalloc.VReg, rd, rn, rm operand, _64bit, signed bool) {
	div := m.allocateInstr()

	if signed {
		div.asALU(aluOpSDiv, rd, rn, rm, _64bit)
	} else {
		div.asALU(aluOpUDiv, rd, rn, rm, _64bit)
	}
	m.insert(div)

	// Check if rm is zero:
	m.exitIfNot(execCtxVReg, registerAsRegNotZeroCond(rm.nr()), wazevoapi.ExitCodeIntegerDivisionByZero)

	// rd = rn-rd*rm by MSUB instruction.
	msub := m.allocateInstr()
	msub.asALURRRR(aluOpMSub, rd, rd, rm, rn, _64bit)
	m.insert(msub)
}

func (m *machine) lowerIDiv(execCtxVReg regalloc.VReg, rd, rn, rm operand, _64bit, signed bool) {
	div := m.allocateInstr()

	if signed {
		div.asALU(aluOpSDiv, rd, rn, rm, _64bit)
	} else {
		div.asALU(aluOpUDiv, rd, rn, rm, _64bit)
	}
	m.insert(div)

	// Check if rm is zero:
	m.exitIfNot(execCtxVReg, registerAsRegNotZeroCond(rm.nr()), wazevoapi.ExitCodeIntegerDivisionByZero)

	if signed {
		// We need to check the signed overflow which happens iff "math.MinInt{32,64} / -1"
		minusOneCheck := m.allocateInstr()
		// Sets eq condition if rm == -1.
		minusOneCheck.asALU(aluOpAddS, operandNR(xzrVReg), rm, operandImm12(1, 0), _64bit)
		m.insert(minusOneCheck)

		ccmp := m.allocateInstr()
		// If eq condition is set, sets the flag by the result based on "rn - 1", otherwise clears the flag.
		ccmp.asCCmpImm(rn, 1, eq, 0, _64bit)
		m.insert(ccmp)

		// Check the overflow flag.
		m.exitIfNot(execCtxVReg, vs.invert().asCond(), wazevoapi.ExitCodeIntegerOverflow)
	}
}

func (m *machine) exitIfNot(execCtxVReg regalloc.VReg, c cond, code wazevoapi.ExitCode) {
	cbr := m.allocateInstr()
	m.insert(cbr)
	m.lowerExitWithCode(execCtxVReg, code)
	// Conditional branch target is after exit.
	l := m.insertBrTargetLabel()
	cbr.asCondBr(c, l, false /* ignored */)
}

func (m *machine) lowerFcopysign(x, y, ret ssa.Value) {
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
	tmpF := operandNR(m.compiler.AllocateVReg(regalloc.RegTypeFloat))

	// This is exactly the same code emitted by GCC for "__builtin_copysign":
	//
	//    mov     x0, -9223372036854775808
	//    fmov    d2, x0
	//    vbit    v0.8b, v1.8b, v2.8b
	//

	setMSB := m.allocateInstr()
	if x.Type() == ssa.TypeF32 {
		m.lowerConstantI32(tmpRegVReg, math.MinInt32)
		setMSB.asMovToVec(tmpF, operandNR(tmpRegVReg), vecArrangementS, vecIndex(0))
	} else {
		m.lowerConstantI64(tmpRegVReg, math.MinInt64)
		setMSB.asMovToVec(tmpF, operandNR(tmpRegVReg), vecArrangementD, vecIndex(0))
	}
	m.insert(setMSB)

	vbit := m.allocateInstr()
	vbit.asVecRRR(vecOpBit, rn, rm, tmpF, vecArrangement8B)
	m.insert(vbit)

	mov := m.allocateInstr()
	mov.asFpuMov64(m.compiler.VRegOf(ret), rn.nr())
	m.insert(mov)
}

func (m *machine) lowerBitcast(instr *ssa.Instruction) {
	v, dstType := instr.BitcastData()
	srcType := v.Type()
	rn := m.getOperand_NR(m.compiler.ValueDefinition(v), extModeNone)
	rd := operandNR(m.compiler.VRegOf(instr.Return()))
	srcInt := srcType.IsInt()
	dstInt := dstType.IsInt()
	switch {
	case srcInt && !dstInt: // Int to Float:
		mov := m.allocateInstr()
		var arr vecArrangement
		if srcType.Bits() == 64 {
			arr = vecArrangementD
		} else {
			arr = vecArrangementS
		}
		mov.asMovToVec(rd, rn, arr, vecIndex(0))
		m.insert(mov)
	case !srcInt && dstInt: // Float to Int:
		mov := m.allocateInstr()
		var arr vecArrangement
		if dstType.Bits() == 64 {
			arr = vecArrangementD
		} else {
			arr = vecArrangementS
		}
		mov.asMovFromVec(rd, rn, arr, vecIndex(0))
		m.insert(mov)
	default:
		panic("TODO?BUG?")
	}
}

func (m *machine) lowerFpuUniOp(op fpuUniOp, in, out ssa.Value) {
	rn := m.getOperand_NR(m.compiler.ValueDefinition(in), extModeNone)
	rd := operandNR(m.compiler.VRegOf(out))

	neg := m.allocateInstr()
	neg.asFpuRR(op, rd, rn, in.Type().Bits() == 64)
	m.insert(neg)
}

func (m *machine) lowerFpuToInt(rd, rn operand, ctx regalloc.VReg, signed, src64bit, dst64bit, nonTrapping bool) {
	if !nonTrapping {
		// First of all, we have to clear the FPU flags.
		flagClear := m.allocateInstr()
		flagClear.asMovToFPSR(xzrVReg)
		m.insert(flagClear)
	}

	// Then, do the conversion which doesn't trap inherently.
	cvt := m.allocateInstr()
	cvt.asFpuToInt(rd, rn, signed, src64bit, dst64bit)
	m.insert(cvt)

	if !nonTrapping {
		// After the conversion, check the FPU flags.
		getFlag := m.allocateInstr()
		getFlag.asMovFromFPSR(tmpRegVReg)
		m.insert(getFlag)

		// Check if the conversion was undefined by comparing the status with 1.
		// See https://developer.arm.com/documentation/ddi0595/2020-12/AArch64-Registers/FPSR--Floating-point-Status-Register
		alu := m.allocateInstr()
		alu.asALU(aluOpSubS, operandNR(xzrVReg), operandNR(tmpRegVReg), operandImm12(1, 0), true)
		m.insert(alu)

		// If it is not undefined, we can return the result.
		ok := m.allocateInstr()
		m.insert(ok)

		// Otherwise, we have to choose the status depending on it is overflow or NaN conversion.

		// Comparing itself to check if it is a NaN.
		fpuCmp := m.allocateInstr()
		fpuCmp.asFpuCmp(rn, rn, src64bit)
		m.insert(fpuCmp)
		// If the VC flag is not set (== VS flag is set), it is a NaN.
		m.exitIfNot(ctx, vc.asCond(), wazevoapi.ExitCodeInvalidConversionToInteger)
		// Otherwise, it is an overflow.
		m.lowerExitWithCode(ctx, wazevoapi.ExitCodeIntegerOverflow)

		// Conditional branch target is after exit.
		l := m.insertBrTargetLabel()
		ok.asCondBr(ne.asCond(), l, false /* ignored */)
	}
}

func (m *machine) lowerIntToFpu(rd, rn operand, signed, src64bit, dst64bit bool) {
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
	x, y := si.Arg2()
	xDef, yDef := m.compiler.ValueDefinition(x), m.compiler.ValueDefinition(y)
	rn := m.getOperand_NR(xDef, extModeNone)
	rm := m.getOperand_NR(yDef, extModeNone)
	rd := operandNR(m.compiler.VRegOf(si.Return()))
	instr.asFpuRRR(op, rd, rn, rm, x.Type().Bits() == 64)
	m.insert(instr)
}

func (m *machine) lowerSubOrAdd(si *ssa.Instruction, add bool) {
	x, y := si.Arg2()
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
func (m *machine) InsertMove(dst, src regalloc.VReg, typ ssa.Type) {
	instr := m.allocateInstr()
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		instr.asMove64(dst, src)
	case ssa.TypeF32, ssa.TypeF64:
		instr.asFpuMov64(dst, src)
	case ssa.TypeV128:
		instr.asFpuMov128(dst, src)
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
	x, amount := si.Arg2()
	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), ext)
	rm := m.getOperand_ShiftImm_NR(m.compiler.ValueDefinition(amount), ext, x.Type().Bits())
	rd := operandNR(m.compiler.VRegOf(si.Return()))

	alu := m.allocateInstr()
	alu.asALUShift(aluOp, rd, rn, rm, x.Type().Bits() == 64)
	m.insert(alu)
}

func (m *machine) lowerBitwiseAluOp(si *ssa.Instruction, op aluOp) {
	x, y := si.Arg2()
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
	x, y := si.Arg2()
	r := si.Return()
	_64 := r.Type().Bits() == 64

	rn := m.getOperand_NR(m.compiler.ValueDefinition(x), extModeNone)
	rm := m.getOperand_NR(m.compiler.ValueDefinition(y), extModeNone)
	rd := operandNR(m.compiler.VRegOf(r))

	// Encode rotl as neg + rotr: neg is a sub against the zero-reg.
	neg := m.allocateInstr()
	neg.asALU(aluOpSub, rm, operandNR(xzrVReg), rm, _64)
	m.insert(neg)
	alu := m.allocateInstr()
	alu.asALU(aluOpRotR, rd, rn, rm, _64)
	m.insert(alu)
}

func (m *machine) lowerRotr(si *ssa.Instruction) {
	x, y := si.Arg2()

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

	// In order to unwind the stack, we also need to push the current stack pointer:
	movSpToTmp := m.allocateInstr()
	movSpToTmp.asMove64(tmpRegVReg, spVReg)
	strSpToExecCtx := m.allocateInstr()
	strSpToExecCtx.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtxVReg, imm: wazevoapi.ExecutionContextOffsets.StackPointerBeforeGoCall.I64(),
		}, 64)
	// Also the address of this exit.
	currentAddrToTmp := m.allocateInstrAfterLowering()
	currentAddrToTmp.asAdr(tmpRegVReg, 0)
	storeCurrentAddrToExecCtx := m.allocateInstrAfterLowering()
	storeCurrentAddrToExecCtx.asStore(operandNR(tmpRegVReg),
		addressMode{
			kind: addressModeKindRegUnsignedImm12,
			rn:   execCtxVReg, imm: wazevoapi.ExecutionContextOffsets.GoCallReturnAddress.I64(),
		}, 64)

	exitSeq := m.allocateInstr()
	exitSeq.asExitSequence(execCtxVReg)

	m.insert(loadExitCodeConst)
	m.insert(setExitCode)
	m.insert(movSpToTmp)
	m.insert(strSpToExecCtx)
	m.insert(currentAddrToTmp)
	m.insert(storeCurrentAddrToExecCtx)
	m.insert(exitSeq)
}

func (m *machine) lowerIcmpToFlag(x, y ssa.Value, signed bool) {
	if x.Type() != y.Type() {
		panic(
			fmt.Sprintf("TODO(maybe): support icmp with different types: v%d=%s != v%d=%s",
				x.ID(), x.Type(), y.ID(), y.Type()))
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
		panic("TODO: OpcodeExitIfTrueWithCode must come after Icmp at the moment: " + condDef.Instr.Opcode().String())
	}
	m.compiler.MarkLowered(condDef.Instr)

	cvalInstr := condDef.Instr
	x, y, c := cvalInstr.IcmpData()
	signed := c.Signed()
	m.lowerIcmpToFlag(x, y, signed)

	// We have to skip the entire exit sequence if the condition is false.
	cbr := m.allocateInstr()
	m.insert(cbr)
	m.lowerExitWithCode(execCtxVReg, code)
	// conditional branch target is after exit.
	l := m.insertBrTargetLabel()
	cbr.asCondBr(condFlagFromSSAIntegerCmpCond(c).invert().asCond(), l, false /* ignored */)
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
