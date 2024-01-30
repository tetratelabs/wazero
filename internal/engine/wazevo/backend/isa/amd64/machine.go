package amd64

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/platform"
)

// NewBackend returns a new backend for arm64.
func NewBackend() backend.Machine {
	ectx := backend.NewExecutableContextT[instruction](
		resetInstruction,
		setNext,
		setPrev,
		asNop,
	)
	return &machine{
		ectx:        ectx,
		cpuFeatures: platform.CpuFeatures,
		regAlloc:    regalloc.NewAllocator(regInfo),
		spillSlots:  map[regalloc.VRegID]int64{},
	}
}

type (
	// machine implements backend.Machine for amd64.
	machine struct {
		c                        backend.Compiler
		ectx                     *backend.ExecutableContextT[instruction]
		stackBoundsCheckDisabled bool

		cpuFeatures platform.CpuFeatureFlags

		regAlloc        regalloc.Allocator
		regAllocFn      *backend.RegAllocFunction[*instruction, *machine]
		regAllocStarted bool

		spillSlotSize int64
		spillSlots    map[regalloc.VRegID]int64
		currentABI    *backend.FunctionABI
		clobberedRegs []regalloc.VReg

		maxRequiredStackSizeForCalls int64

		labelResolutionPends []labelResolutionPend
	}

	labelResolutionPend struct {
		instr       *instruction
		instrOffset int64
		// imm32Offset is the offset of the last 4 bytes of the instruction.
		imm32Offset int64
	}
)

// Reset implements backend.Machine.
func (m *machine) Reset() {
	m.clobberedRegs = m.clobberedRegs[:0]
	for key := range m.spillSlots {
		m.clobberedRegs = append(m.clobberedRegs, regalloc.VReg(key))
	}
	for _, key := range m.clobberedRegs {
		delete(m.spillSlots, regalloc.VRegID(key))
	}

	m.stackBoundsCheckDisabled = false
	m.ectx.Reset()

	m.regAllocFn.Reset()
	m.regAlloc.Reset()
	m.regAllocStarted = false
	m.clobberedRegs = m.clobberedRegs[:0]

	m.spillSlotSize = 0
	m.maxRequiredStackSizeForCalls = 0
}

// ExecutableContext implements backend.Machine.
func (m *machine) ExecutableContext() backend.ExecutableContext { return m.ectx }

// DisableStackCheck implements backend.Machine.
func (m *machine) DisableStackCheck() { m.stackBoundsCheckDisabled = true }

// SetCompiler implements backend.Machine.
func (m *machine) SetCompiler(c backend.Compiler) {
	m.c = c
	m.regAllocFn = backend.NewRegAllocFunction[*instruction, *machine](m, c.SSABuilder(), c)
}

// SetCurrentABI implements backend.Machine.
func (m *machine) SetCurrentABI(abi *backend.FunctionABI) {
	m.currentABI = abi
}

// RegAlloc implements backend.Machine.
func (m *machine) RegAlloc() {
	rf := m.regAllocFn
	for _, pos := range m.ectx.OrderedBlockLabels {
		rf.AddBlock(pos.SB, pos.L, pos.Begin, pos.End)
	}

	m.regAllocStarted = true
	m.regAlloc.DoAllocation(rf)
	// Now that we know the final spill slot size, we must align spillSlotSize to 16 bytes.
	m.spillSlotSize = (m.spillSlotSize + 15) &^ 15
}

// InsertReturn implements backend.Machine.
func (m *machine) InsertReturn() {
	i := m.allocateInstr().asRet(m.currentABI)
	m.insert(i)
}

// LowerSingleBranch implements backend.Machine.
func (m *machine) LowerSingleBranch(b *ssa.Instruction) {
	ectx := m.ectx
	switch b.Opcode() {
	case ssa.OpcodeJump:
		_, _, targetBlk := b.BranchData()
		if b.IsFallthroughJump() {
			return
		}
		jmp := m.allocateInstr()
		target := ectx.GetOrAllocateSSABlockLabel(targetBlk)
		if target == backend.LabelReturn {
			jmp.asRet(m.currentABI)
		} else {
			jmp.asJmp(newOperandLabel(target))
		}
		m.insert(jmp)
	case ssa.OpcodeBrTable:
		index, target := b.BrTableData()
		m.lowerBrTable(index, target)
	default:
		panic("BUG: unexpected branch opcode" + b.Opcode().String())
	}
}

var condBranchMatches = [...]ssa.Opcode{ssa.OpcodeIcmp, ssa.OpcodeFcmp}

func (m *machine) lowerBrTable(index ssa.Value, targets []ssa.BasicBlock) {
	_v := m.getOperand_Reg(m.c.ValueDefinition(index))
	v := m.copyToTmp(_v.r)

	// First, we need to do the bounds check.
	maxIndex := m.c.AllocateVReg(ssa.TypeI32)
	m.lowerIconst(maxIndex, uint64(len(targets)-1), false)
	cmp := m.allocateInstr().asCmpRmiR(true, newOperandReg(maxIndex), v, false)
	m.insert(cmp)

	// Then do the conditional move maxIndex to v if v > maxIndex.
	cmov := m.allocateInstr().asCmove(condNB, newOperandReg(maxIndex), v, false)
	m.insert(cmov)

	// Now that v has the correct index. Load the address of the jump table into the addr.
	addr := m.c.AllocateVReg(ssa.TypeI64)
	leaJmpTableAddr := m.allocateInstr()
	m.insert(leaJmpTableAddr)

	// Then add the target's offset into jmpTableAddr.
	loadTargetOffsetFromJmpTable := m.allocateInstr().asAluRmiR(aluRmiROpcodeAdd,
		// Shift by 3 because each entry is 8 bytes.
		newOperandMem(newAmodeRegRegShift(0, addr, v, 3)), addr, true)
	m.insert(loadTargetOffsetFromJmpTable)

	// Now ready to jump.
	jmp := m.allocateInstr().asJmp(newOperandReg(addr))
	m.insert(jmp)

	jmpTableBegin, jmpTableBeginLabel := m.allocateBrTarget()
	m.insert(jmpTableBegin)
	leaJmpTableAddr.asLEA(newAmodeRipRelative(jmpTableBeginLabel), addr)

	jmpTable := m.allocateInstr()
	// TODO: reuse the slice!
	labels := make([]uint32, len(targets))
	for j, target := range targets {
		labels[j] = uint32(m.ectx.GetOrAllocateSSABlockLabel(target))
	}

	jmpTable.asJmpTableSequence(labels)
	m.insert(jmpTable)
}

// LowerConditionalBranch implements backend.Machine.
func (m *machine) LowerConditionalBranch(b *ssa.Instruction) {
	exctx := m.ectx
	cval, args, targetBlk := b.BranchData()
	if len(args) > 0 {
		panic(fmt.Sprintf(
			"conditional branch shouldn't have args; likely a bug in critical edge splitting: from %s to %s",
			exctx.CurrentSSABlk,
			targetBlk,
		))
	}

	target := exctx.GetOrAllocateSSABlockLabel(targetBlk)
	cvalDef := m.c.ValueDefinition(cval)

	switch m.c.MatchInstrOneOf(cvalDef, condBranchMatches[:]) {
	case ssa.OpcodeIcmp:
		cvalInstr := cvalDef.Instr
		x, y, c := cvalInstr.IcmpData()

		cc := condFromSSAIntCmpCond(c)
		if b.Opcode() == ssa.OpcodeBrz {
			cc = cc.invert()
		}

		// First, perform the comparison and set the flag.
		xd, yd := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
		m.lowerIcmpToFlag(xd, yd, x.Type() == ssa.TypeI64)

		// Then perform the conditional branch.
		m.insert(m.allocateInstr().asJmpIf(cc, newOperandLabel(target)))
		cvalDef.Instr.MarkLowered()
	case ssa.OpcodeFcmp:
		cvalInstr := cvalDef.Instr

		f1, f2, and := m.lowerFcmpToFlags(cvalInstr)
		if f2 == condInvalid {
			m.insert(m.allocateInstr().asJmpIf(f1, newOperandLabel(target)))
		} else {
			jmp1, jmp2 := m.allocateInstr(), m.allocateInstr()
			m.insert(jmp1)
			m.insert(jmp2)
			notTaken, notTakenLabel := m.allocateBrTarget()
			m.insert(notTaken)
			if and {
				jmp1.asJmpIf(f1.invert(), newOperandLabel(notTakenLabel))
				jmp2.asJmpIf(f2, newOperandLabel(target))
			} else {
				jmp1.asJmpIf(f1, newOperandLabel(target))
				jmp2.asJmpIf(f2, newOperandLabel(target))
			}
		}

		cvalDef.Instr.MarkLowered()
	default:
		v := m.getOperand_Reg(cvalDef)

		var cc cond
		if b.Opcode() == ssa.OpcodeBrz {
			cc = condZ
		} else {
			cc = condNZ
		}

		// Perform test %v, %v to set the flag.
		cmp := m.allocateInstr().asCmpRmiR(false, v, v.r, false)
		m.insert(cmp)
		m.insert(m.allocateInstr().asJmpIf(cc, newOperandLabel(target)))
	}
}

// LowerInstr implements backend.Machine.
func (m *machine) LowerInstr(instr *ssa.Instruction) {
	switch op := instr.Opcode(); op {
	case ssa.OpcodeBrz, ssa.OpcodeBrnz, ssa.OpcodeJump, ssa.OpcodeBrTable:
		panic("BUG: branching instructions are handled by LowerBranches")
	case ssa.OpcodeReturn:
		panic("BUG: return must be handled by backend.Compiler")
	case ssa.OpcodeIconst, ssa.OpcodeF32const, ssa.OpcodeF64const: // Constant instructions are inlined.
	case ssa.OpcodeCall, ssa.OpcodeCallIndirect:
		m.lowerCall(instr)
	case ssa.OpcodeStore, ssa.OpcodeIstore8, ssa.OpcodeIstore16, ssa.OpcodeIstore32:
		m.lowerStore(instr)
	case ssa.OpcodeIadd:
		m.lowerAluRmiROp(instr, aluRmiROpcodeAdd)
	case ssa.OpcodeIsub:
		m.lowerAluRmiROp(instr, aluRmiROpcodeSub)
	case ssa.OpcodeImul:
		m.lowerAluRmiROp(instr, aluRmiROpcodeMul)
	case ssa.OpcodeSdiv, ssa.OpcodeUdiv, ssa.OpcodeSrem, ssa.OpcodeUrem:
		isDiv := op == ssa.OpcodeSdiv || op == ssa.OpcodeUdiv
		isSigned := op == ssa.OpcodeSdiv || op == ssa.OpcodeSrem
		m.lowerIDivRem(instr, isDiv, isSigned)
	case ssa.OpcodeBand:
		m.lowerAluRmiROp(instr, aluRmiROpcodeAnd)
	case ssa.OpcodeBor:
		m.lowerAluRmiROp(instr, aluRmiROpcodeOr)
	case ssa.OpcodeBxor:
		m.lowerAluRmiROp(instr, aluRmiROpcodeXor)
	case ssa.OpcodeIshl:
		m.lowerShiftR(instr, shiftROpShiftLeft)
	case ssa.OpcodeSshr:
		m.lowerShiftR(instr, shiftROpShiftRightArithmetic)
	case ssa.OpcodeUshr:
		m.lowerShiftR(instr, shiftROpShiftRightLogical)
	case ssa.OpcodeRotl:
		m.lowerShiftR(instr, shiftROpRotateLeft)
	case ssa.OpcodeRotr:
		m.lowerShiftR(instr, shiftROpRotateRight)
	case ssa.OpcodeClz:
		m.lowerClz(instr)
	case ssa.OpcodeCtz:
		m.lowerCtz(instr)
	case ssa.OpcodePopcnt:
		m.lowerUnaryRmR(instr, unaryRmROpcodePopcnt)
	case ssa.OpcodeFadd, ssa.OpcodeFsub, ssa.OpcodeFmul, ssa.OpcodeFdiv:
		m.lowerXmmRmR(instr)
	case ssa.OpcodeFabs:
		m.lowerFabsFneg(instr)
	case ssa.OpcodeFneg:
		m.lowerFabsFneg(instr)
	case ssa.OpcodeCeil:
		m.lowerRound(instr, roundingModeUp)
	case ssa.OpcodeFloor:
		m.lowerRound(instr, roundingModeDown)
	case ssa.OpcodeTrunc:
		m.lowerRound(instr, roundingModeZero)
	case ssa.OpcodeNearest:
		m.lowerRound(instr, roundingModeNearest)
	case ssa.OpcodeFmin, ssa.OpcodeFmax:
		m.lowerFminFmax(instr)
	case ssa.OpcodeSqrt:
		m.lowerSqrt(instr)
	case ssa.OpcodeUndefined:
		m.insert(m.allocateInstr().asUD2())
	case ssa.OpcodeExitWithCode:
		execCtx, code := instr.ExitWithCodeData()
		m.lowerExitWithCode(m.c.VRegOf(execCtx), code)
	case ssa.OpcodeExitIfTrueWithCode:
		execCtx, c, code := instr.ExitIfTrueWithCodeData()
		m.lowerExitIfTrueWithCode(m.c.VRegOf(execCtx), c, code)
	case ssa.OpcodeLoad:
		ptr, offset, typ := instr.LoadData()
		dst := m.c.VRegOf(instr.Return())
		m.lowerLoad(ptr, offset, typ, dst)
	case ssa.OpcodeUload8, ssa.OpcodeUload16, ssa.OpcodeUload32, ssa.OpcodeSload8, ssa.OpcodeSload16, ssa.OpcodeSload32:
		ptr, offset, _ := instr.LoadData()
		ret := m.c.VRegOf(instr.Return())
		m.lowerExtLoad(op, ptr, offset, ret)
	case ssa.OpcodeVconst:
		result := instr.Return()
		lo, hi := instr.VconstData()
		m.lowerVconst(result, lo, hi)
	case ssa.OpcodeSExtend, ssa.OpcodeUExtend:
		from, to, signed := instr.ExtendData()
		m.lowerExtend(instr.Arg(), instr.Return(), from, to, signed)
	case ssa.OpcodeIcmp:
		m.lowerIcmp(instr)
	case ssa.OpcodeFcmp:
		m.lowerFcmp(instr)
	case ssa.OpcodeSelect:
		cval, x, y := instr.SelectData()
		m.lowerSelect(x, y, cval, instr.Return())
	case ssa.OpcodeIreduce:
		rn := m.getOperand_Mem_Reg(m.c.ValueDefinition(instr.Arg()))
		retVal := instr.Return()
		rd := m.c.VRegOf(retVal)

		if retVal.Type() != ssa.TypeI32 {
			panic("TODO?: Ireduce to non-i32")
		}
		m.insert(m.allocateInstr().asMovzxRmR(extModeLQ, rn, rd))

	default:
		panic("TODO: lowering " + op.String())
	}
}

func (m *machine) lowerFcmp(instr *ssa.Instruction) {
	f1, f2, and := m.lowerFcmpToFlags(instr)
	rd := m.c.VRegOf(instr.Return())
	if f2 == condInvalid {
		tmp := m.c.AllocateVReg(ssa.TypeI32)
		m.insert(m.allocateInstr().asSetcc(f1, tmp))
		// On amd64, setcc only sets the first byte of the register, so we need to zero extend it to match
		// the semantics of Icmp that sets either 0 or 1.
		m.insert(m.allocateInstr().asMovzxRmR(extModeBQ, newOperandReg(tmp), rd))
	} else {
		tmp1, tmp2 := m.c.AllocateVReg(ssa.TypeI32), m.c.AllocateVReg(ssa.TypeI32)
		m.insert(m.allocateInstr().asSetcc(f1, tmp1))
		m.insert(m.allocateInstr().asSetcc(f2, tmp2))
		var op aluRmiROpcode
		if and {
			op = aluRmiROpcodeAnd
		} else {
			op = aluRmiROpcodeOr
		}
		m.insert(m.allocateInstr().asAluRmiR(op, newOperandReg(tmp1), tmp2, false))
		m.insert(m.allocateInstr().asMovzxRmR(extModeBQ, newOperandReg(tmp2), rd))
	}
}

func (m *machine) lowerIcmp(instr *ssa.Instruction) {
	x, y, c := instr.IcmpData()
	m.lowerIcmpToFlag(m.c.ValueDefinition(x), m.c.ValueDefinition(y), x.Type() == ssa.TypeI64)
	rd := m.c.VRegOf(instr.Return())
	tmp := m.c.AllocateVReg(ssa.TypeI32)
	m.insert(m.allocateInstr().asSetcc(condFromSSAIntCmpCond(c), tmp))
	// On amd64, setcc only sets the first byte of the register, so we need to zero extend it to match
	// the semantics of Icmp that sets either 0 or 1.
	m.insert(m.allocateInstr().asMovzxRmR(extModeBQ, newOperandReg(tmp), rd))
}

func (m *machine) lowerSelect(x, y, cval, ret ssa.Value) {
	xo, yo := m.getOperand_Mem_Reg(m.c.ValueDefinition(x)), m.getOperand_Reg(m.c.ValueDefinition(y))
	rd := m.c.VRegOf(ret)

	var cond cond
	cvalDef := m.c.ValueDefinition(cval)
	switch m.c.MatchInstrOneOf(cvalDef, condBranchMatches[:]) {
	case ssa.OpcodeIcmp:
		icmp := cvalDef.Instr
		xc, yc, cc := icmp.IcmpData()
		m.lowerIcmpToFlag(m.c.ValueDefinition(xc), m.c.ValueDefinition(yc), xc.Type() == ssa.TypeI64)
		cond = condFromSSAIntCmpCond(cc)
		icmp.Lowered()
	default: // TODO: match ssa.OpcodeFcmp for optimization, but seems a bit complex.
		cv := m.getOperand_Reg(cvalDef).r
		test := m.allocateInstr().asCmpRmiR(false, newOperandReg(cv), cv, false)
		m.insert(test)
		cond = condNZ
	}

	if typ := x.Type(); typ.IsInt() {
		_64 := typ.Bits() == 64
		mov := m.allocateInstr()
		tmp := m.c.AllocateVReg(typ)
		switch yo.kind {
		case operandKindReg:
			mov.asMovRR(yo.r, tmp, _64)
		case operandKindMem:
			if _64 {
				mov.asMov64MR(yo, tmp)
			} else {
				mov.asMovzxRmR(extModeLQ, yo, tmp)
			}
		default:
			panic("BUG")
		}
		m.insert(mov)
		cmov := m.allocateInstr().asCmove(cond, xo, tmp, _64)
		m.insert(cmov)
		m.insert(m.allocateInstr().asMovRR(tmp, rd, _64))
	} else {
		mov := m.allocateInstr()
		tmp := m.c.AllocateVReg(typ)
		switch typ {
		case ssa.TypeF32:
			mov.asXmmUnaryRmR(sseOpcodeMovss, yo, tmp)
		case ssa.TypeF64:
			mov.asXmmUnaryRmR(sseOpcodeMovsd, yo, tmp)
		case ssa.TypeV128:
			mov.asXmmUnaryRmR(sseOpcodeMovdqu, yo, tmp)
		default:
			panic("BUG")
		}
		m.insert(mov)

		jcc := m.allocateInstr()
		m.insert(jcc)

		cmov := m.allocateInstr()
		m.insert(cmov)
		switch typ {
		case ssa.TypeF32:
			cmov.asXmmUnaryRmR(sseOpcodeMovss, xo, tmp)
		case ssa.TypeF64:
			cmov.asXmmUnaryRmR(sseOpcodeMovsd, xo, tmp)
		case ssa.TypeV128:
			cmov.asXmmUnaryRmR(sseOpcodeMovdqu, xo, tmp)
		default:
			panic("BUG")
		}

		nop, end := m.allocateBrTarget()
		m.insert(nop)

		jcc.asJmpIf(cond.invert(), newOperandLabel(end))
		m.insert(m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandReg(tmp), rd))
	}
}

func (m *machine) lowerExtend(_arg, ret ssa.Value, from, to byte, signed bool) {
	rd := m.c.VRegOf(ret)
	arg := m.getOperand_Mem_Reg(m.c.ValueDefinition(_arg))

	ext := m.allocateInstr()
	switch {
	case from == 8 && to == 16 && signed:
		ext.asMovsxRmR(extModeBQ, arg, rd)
	case from == 8 && to == 16 && !signed:
		ext.asMovzxRmR(extModeBQ, arg, rd)
	case from == 8 && to == 32 && signed:
		ext.asMovsxRmR(extModeBQ, arg, rd)
	case from == 8 && to == 32 && !signed:
		ext.asMovzxRmR(extModeBQ, arg, rd)
	case from == 8 && to == 64 && signed:
		ext.asMovsxRmR(extModeBQ, arg, rd)
	case from == 8 && to == 64 && !signed:
		ext.asMovzxRmR(extModeBQ, arg, rd)
	case from == 16 && to == 32 && signed:
		ext.asMovsxRmR(extModeWQ, arg, rd)
	case from == 16 && to == 32 && !signed:
		ext.asMovzxRmR(extModeWQ, arg, rd)
	case from == 16 && to == 64 && signed:
		ext.asMovsxRmR(extModeWQ, arg, rd)
	case from == 16 && to == 64 && !signed:
		ext.asMovzxRmR(extModeWQ, arg, rd)
	case from == 32 && to == 64 && signed:
		ext.asMovsxRmR(extModeLQ, arg, rd)
	case from == 32 && to == 64 && !signed:
		ext.asMovzxRmR(extModeLQ, arg, rd)
	default:
		panic(fmt.Sprintf("BUG: unhandled extend: from=%d, to=%d, signed=%t", from, to, signed))
	}
	m.insert(ext)
}

func (m *machine) lowerVconst(res ssa.Value, lo, hi uint64) {
	// TODO: use xor when lo == hi == 0.

	dst := m.c.VRegOf(res)

	islandAddr := m.c.AllocateVReg(ssa.TypeI64)
	lea := m.allocateInstr()
	load := m.allocateInstr().asXmmUnaryRmR(sseOpcodeMovdqu, newOperandMem(newAmodeImmReg(0, islandAddr)), dst)
	jmp := m.allocateInstr()

	constLabelNop, constLabel := m.allocateBrTarget()
	constIsland := m.allocateInstr().asV128ConstIsland(lo, hi)
	afterLoadNop, afterLoadLabel := m.allocateBrTarget()

	// 		lea    constLabel(%rip), %islandAddr
	// 		movdqu (%islandAddr), %dst
	//		jmp    afterConst
	// constLabel:
	//		constIsland $lo, $hi
	// afterConst:

	m.insert(lea)
	m.insert(load)
	m.insert(jmp)
	m.insert(constLabelNop)
	m.insert(constIsland)
	m.insert(afterLoadNop)

	lea.asLEA(newAmodeRipRelative(constLabel), islandAddr)
	jmp.asJmp(newOperandLabel(afterLoadLabel))
}

func (m *machine) lowerCtz(instr *ssa.Instruction) {
	if m.cpuFeatures.HasExtra(platform.CpuExtraFeatureAmd64ABM) {
		m.lowerUnaryRmR(instr, unaryRmROpcodeTzcnt)
	} else {
		// On processors that do not support TZCNT, the BSF instruction is
		// executed instead. The key difference between TZCNT and BSF
		// instruction is that if source operand is zero, the content of
		// destination operand is undefined.
		// https://www.felixcloutier.com/x86/tzcnt.html

		x := instr.Arg()
		if !x.Type().IsInt() {
			panic("BUG?")
		}
		_64 := x.Type().Bits() == 64

		xDef := m.c.ValueDefinition(x)
		rm := m.getOperand_Reg(xDef)
		rd := m.c.VRegOf(instr.Return())

		// First, we have to check if the target is non-zero.
		test := m.allocateInstr()
		test.asCmpRmiR(false, rm, rm.r, _64)
		m.insert(test)

		jmpNz := m.allocateInstr() // Will backpatch the operands later.
		m.insert(jmpNz)

		// If the value is zero, we just push the const value.
		m.lowerIconst(rd, uint64(x.Type().Bits()), _64)

		// Now jump right after the non-zero case.
		jmpAtEnd := m.allocateInstr() // Will backpatch later.
		m.insert(jmpAtEnd)

		// jmpNz target label is set here.
		nop, nz := m.allocateBrTarget()
		jmpNz.asJmpIf(condNZ, newOperandLabel(nz))
		m.insert(nop)

		// Emit the non-zero case.
		bsr := m.allocateInstr()
		bsr.asUnaryRmR(unaryRmROpcodeBsf, rm, rd, _64)
		m.insert(bsr)

		// jmpAtEnd target label is set here.
		nopEnd, end := m.allocateBrTarget()
		jmpAtEnd.asJmp(newOperandLabel(end))
		m.insert(nopEnd)
	}
}

func (m *machine) lowerClz(instr *ssa.Instruction) {
	if m.cpuFeatures.HasExtra(platform.CpuExtraFeatureAmd64ABM) {
		m.lowerUnaryRmR(instr, unaryRmROpcodeLzcnt)
	} else {
		// On processors that do not support LZCNT, we combine BSR (calculating
		// most significant set bit) with XOR. This logic is described in
		// "Replace Raw Assembly Code with Builtin Intrinsics" section in:
		// https://developer.apple.com/documentation/apple-silicon/addressing-architectural-differences-in-your-macos-code.

		x := instr.Arg()
		if !x.Type().IsInt() {
			panic("BUG?")
		}
		_64 := x.Type().Bits() == 64

		xDef := m.c.ValueDefinition(x)
		rm := m.getOperand_Reg(xDef)
		rd := m.c.VRegOf(instr.Return())

		// First, we have to check if the rm is non-zero as BSR is undefined
		// on zero. See https://www.felixcloutier.com/x86/bsr.
		test := m.allocateInstr()
		test.asCmpRmiR(false, rm, rm.r, _64)
		m.insert(test)

		jmpNz := m.allocateInstr() // Will backpatch later.
		m.insert(jmpNz)

		// If the value is zero, we just push the const value.
		m.lowerIconst(rd, uint64(x.Type().Bits()), _64)

		// Now jump right after the non-zero case.
		jmpAtEnd := m.allocateInstr() // Will backpatch later.
		m.insert(jmpAtEnd)

		// jmpNz target label is set here.
		nop, nz := m.allocateBrTarget()
		jmpNz.asJmpIf(condNZ, newOperandLabel(nz))
		m.insert(nop)

		// Emit the non-zero case.
		tmp := m.c.VRegOf(instr.Return())
		bsr := m.allocateInstr()
		bsr.asUnaryRmR(unaryRmROpcodeBsr, rm, tmp, _64)
		m.insert(bsr)

		// Now we XOR the value with the bit length minus one.
		xor := m.allocateInstr()
		xor.asAluRmiR(aluRmiROpcodeXor, newOperandImm32(uint32(x.Type().Bits()-1)), tmp, _64)
		m.insert(xor)

		// jmpAtEnd target label is set here.
		nopEnd, end := m.allocateBrTarget()
		jmpAtEnd.asJmp(newOperandLabel(end))
		m.insert(nopEnd)
	}
}

func (m *machine) lowerUnaryRmR(si *ssa.Instruction, op unaryRmROpcode) {
	x := si.Arg()
	if !x.Type().IsInt() {
		panic("BUG?")
	}
	_64 := x.Type().Bits() == 64

	xDef := m.c.ValueDefinition(x)
	rm := m.getOperand_Mem_Reg(xDef)
	rd := m.c.VRegOf(si.Return())

	instr := m.allocateInstr()
	instr.asUnaryRmR(op, rm, rd, _64)
	m.insert(instr)
}

func (m *machine) lowerLoad(ptr ssa.Value, offset uint32, typ ssa.Type, dst regalloc.VReg) {
	mem := newOperandMem(m.lowerToAddressMode(ptr, offset))
	load := m.allocateInstr()
	switch typ {
	case ssa.TypeI32:
		load.asMovzxRmR(extModeLQ, mem, dst)
	case ssa.TypeI64:
		load.asMov64MR(mem, dst)
	case ssa.TypeF32:
		load.asXmmUnaryRmR(sseOpcodeMovss, mem, dst)
	case ssa.TypeF64:
		load.asXmmUnaryRmR(sseOpcodeMovsd, mem, dst)
	case ssa.TypeV128:
		load.asXmmUnaryRmR(sseOpcodeMovdqu, mem, dst)
	default:
		panic("BUG")
	}
	m.insert(load)
}

func (m *machine) lowerExtLoad(op ssa.Opcode, ptr ssa.Value, offset uint32, dst regalloc.VReg) {
	mem := newOperandMem(m.lowerToAddressMode(ptr, offset))
	load := m.allocateInstr()
	switch op {
	case ssa.OpcodeUload8:
		load.asMovzxRmR(extModeBQ, mem, dst)
	case ssa.OpcodeUload16:
		load.asMovzxRmR(extModeWQ, mem, dst)
	case ssa.OpcodeUload32:
		load.asMovzxRmR(extModeLQ, mem, dst)
	case ssa.OpcodeSload8:
		load.asMovsxRmR(extModeBQ, mem, dst)
	case ssa.OpcodeSload16:
		load.asMovsxRmR(extModeWQ, mem, dst)
	case ssa.OpcodeSload32:
		load.asMovsxRmR(extModeLQ, mem, dst)
	default:
		panic("BUG")
	}
	m.insert(load)
}

func (m *machine) lowerExitIfTrueWithCode(execCtx regalloc.VReg, cond ssa.Value, code wazevoapi.ExitCode) {
	condDef := m.c.ValueDefinition(cond)
	if !m.c.MatchInstr(condDef, ssa.OpcodeIcmp) {
		panic("TODO: ExitIfTrue must come after Icmp at the moment: " + condDef.Instr.Opcode().String())
	}
	cvalInstr := condDef.Instr
	cvalInstr.MarkLowered()

	// We need to copy the execution context to a temp register, because if it's spilled,
	// it might end up being reloaded inside the exiting branch.
	execCtxTmp := m.copyToTmp(execCtx)

	x, y, c := cvalInstr.IcmpData()
	m.lowerIcmpToFlag(m.c.ValueDefinition(x), m.c.ValueDefinition(y), x.Type() == ssa.TypeI64)

	jmpIf := m.allocateInstr()
	m.insert(jmpIf)
	l := m.lowerExitWithCode(execCtxTmp, code)
	jmpIf.asJmpIf(condFromSSAIntCmpCond(c).invert(), newOperandLabel(l))
}

func (m *machine) allocateExitInstructions(execCtx, exitCodeReg regalloc.VReg) (setExitCode, saveRsp, saveRbp *instruction) {
	setExitCode = m.allocateInstr().asMovRM(
		exitCodeReg,
		newOperandMem(newAmodeImmReg(wazevoapi.ExecutionContextOffsetExitCodeOffset.U32(), execCtx)),
		4,
	)
	saveRsp = m.allocateInstr().asMovRM(
		rspVReg,
		newOperandMem(newAmodeImmReg(wazevoapi.ExecutionContextOffsetStackPointerBeforeGoCall.U32(), execCtx)),
		8,
	)

	saveRbp = m.allocateInstr().asMovRM(
		rbpVReg,
		newOperandMem(newAmodeImmReg(wazevoapi.ExecutionContextOffsetFramePointerBeforeGoCall.U32(), execCtx)),
		8,
	)
	return
}

func (m *machine) lowerExitWithCode(execCtx regalloc.VReg, code wazevoapi.ExitCode) (afterLabel backend.Label) {
	// First we set the exit code in the execution context.
	exitCodeReg := m.c.AllocateVReg(ssa.TypeI32)
	m.lowerIconst(exitCodeReg, uint64(code), false)

	setExitCode, saveRsp, saveRbp := m.allocateExitInstructions(execCtx, exitCodeReg)

	// Set exit code, save RSP and RBP.
	m.insert(setExitCode)
	m.insert(saveRsp)
	m.insert(saveRbp)

	// Next is to save the return address.
	readRip := m.allocateInstr()
	m.insert(readRip)
	ripReg := m.c.AllocateVReg(ssa.TypeI64)
	saveRip := m.allocateInstr().asMovRM(
		ripReg,
		newOperandMem(newAmodeImmReg(wazevoapi.ExecutionContextOffsetGoCallReturnAddress.U32(), execCtx)),
		8,
	)
	m.insert(saveRip)

	// Finally exit.
	exitSq := m.allocateInstr().asExitSeq(execCtx)
	m.insert(exitSq)

	// Insert the label for the return address.
	nop, l := m.allocateBrTarget()
	readRip.asLEA(newAmodeRipRelative(l), ripReg)
	m.insert(nop)
	return l
}

func (m *machine) lowerAluRmiROp(si *ssa.Instruction, op aluRmiROpcode) {
	x, y := si.Arg2()
	if !x.Type().IsInt() {
		panic("BUG?")
	}

	_64 := x.Type().Bits() == 64

	xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)

	// TODO: commutative args can be swapped if one of them is an immediate.
	rn := m.getOperand_Reg(xDef)
	rm := m.getOperand_Mem_Imm32_Reg(yDef)
	rd := m.c.VRegOf(si.Return())

	// rn is being overwritten, so we first copy its value to a temp register,
	// in case it is referenced again later.
	tmp := m.copyToTmp(rn.r)

	alu := m.allocateInstr()
	alu.asAluRmiR(op, rm, tmp, _64)
	m.insert(alu)

	// tmp now contains the result, we copy it to the dest register.
	m.copyTo(tmp, rd)
}

func (m *machine) lowerShiftR(si *ssa.Instruction, op shiftROp) {
	x, amt := si.Arg2()
	if !x.Type().IsInt() {
		panic("BUG?")
	}
	_64 := x.Type().Bits() == 64

	xDef, amtDef := m.c.ValueDefinition(x), m.c.ValueDefinition(amt)

	opAmt := m.getOperand_Imm32_Reg(amtDef)
	rx := m.getOperand_Reg(xDef)
	rd := m.c.VRegOf(si.Return())

	// rx is being overwritten, so we first copy its value to a temp register,
	// in case it is referenced again later.
	tmpDst := m.copyToTmp(rx.r)

	if opAmt.r != regalloc.VRegInvalid {
		// If opAmt is a register we must copy its value to rcx,
		// because shiftR encoding mandates that the shift amount is in rcx.
		m.copyTo(opAmt.r, rcxVReg)

		alu := m.allocateInstr()
		alu.asShiftR(op, newOperandReg(rcxVReg), tmpDst, _64)
		m.insert(alu)

	} else {
		alu := m.allocateInstr()
		alu.asShiftR(op, opAmt, tmpDst, _64)
		m.insert(alu)
	}

	// tmp now contains the result, we copy it to the dest register.
	m.copyTo(tmpDst, rd)
}

func (m *machine) lowerXmmRmR(instr *ssa.Instruction) {
	x, y := instr.Arg2()
	if !x.Type().IsFloat() {
		panic("BUG?")
	}
	_64 := x.Type().Bits() == 64

	var op sseOpcode
	if _64 {
		switch instr.Opcode() {
		case ssa.OpcodeFadd:
			op = sseOpcodeAddsd
		case ssa.OpcodeFsub:
			op = sseOpcodeSubsd
		case ssa.OpcodeFmul:
			op = sseOpcodeMulsd
		case ssa.OpcodeFdiv:
			op = sseOpcodeDivsd
		default:
			panic("BUG")
		}
	} else {
		switch instr.Opcode() {
		case ssa.OpcodeFadd:
			op = sseOpcodeAddss
		case ssa.OpcodeFsub:
			op = sseOpcodeSubss
		case ssa.OpcodeFmul:
			op = sseOpcodeMulss
		case ssa.OpcodeFdiv:
			op = sseOpcodeDivss
		default:
			panic("BUG")
		}
	}

	xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
	rn := m.getOperand_Mem_Reg(yDef)
	rm := m.getOperand_Reg(xDef)
	rd := m.c.VRegOf(instr.Return())

	// rm is being overwritten, so we first copy its value to a temp register,
	// in case it is referenced again later.
	tmp := m.copyToTmp(rm.r)

	xmm := m.allocateInstr().asXmmRmR(op, rn, tmp)
	m.insert(xmm)

	m.copyTo(tmp, rd)
}

func (m *machine) lowerSqrt(instr *ssa.Instruction) {
	x := instr.Arg()
	if !x.Type().IsFloat() {
		panic("BUG")
	}
	_64 := x.Type().Bits() == 64
	var op sseOpcode
	if _64 {
		op = sseOpcodeSqrtsd
	} else {
		op = sseOpcodeSqrtss
	}

	xDef := m.c.ValueDefinition(x)
	rm := m.getOperand_Mem_Reg(xDef)
	rd := m.c.VRegOf(instr.Return())

	xmm := m.allocateInstr().asXmmUnaryRmR(op, rm, rd)
	m.insert(xmm)
}

func (m *machine) lowerFabsFneg(instr *ssa.Instruction) {
	x := instr.Arg()
	if !x.Type().IsFloat() {
		panic("BUG")
	}
	_64 := x.Type().Bits() == 64
	var op sseOpcode
	var mask uint64
	if _64 {
		switch instr.Opcode() {
		case ssa.OpcodeFabs:
			mask, op = 0x7fffffffffffffff, sseOpcodeAndpd
		case ssa.OpcodeFneg:
			mask, op = 0x8000000000000000, sseOpcodeXorpd
		}
	} else {
		switch instr.Opcode() {
		case ssa.OpcodeFabs:
			mask, op = 0x7fffffff, sseOpcodeAndps
		case ssa.OpcodeFneg:
			mask, op = 0x80000000, sseOpcodeXorps
		}
	}

	tmp := m.c.AllocateVReg(x.Type())

	xDef := m.c.ValueDefinition(x)
	rm := m.getOperand_Reg(xDef)
	rd := m.c.VRegOf(instr.Return())

	m.lowerFconst(tmp, mask, _64)

	xmm := m.allocateInstr().asXmmRmR(op, rm, tmp)
	m.insert(xmm)

	m.copyTo(tmp, rd)
}

func (m *machine) lowerStore(si *ssa.Instruction) {
	value, ptr, offset, storeSizeInBits := si.StoreData()
	rm := m.getOperand_Reg(m.c.ValueDefinition(value))
	mem := newOperandMem(m.lowerToAddressMode(ptr, offset))

	store := m.allocateInstr()
	switch value.Type() {
	case ssa.TypeI32:
		store.asMovRM(rm.r, mem, storeSizeInBits/8)
	case ssa.TypeI64:
		store.asMovRM(rm.r, mem, storeSizeInBits/8)
	case ssa.TypeF32:
		store.asXmmMovRM(sseOpcodeMovss, rm.r, mem)
	case ssa.TypeF64:
		store.asXmmMovRM(sseOpcodeMovsd, rm.r, mem)
	case ssa.TypeV128:
		store.asXmmMovRM(sseOpcodeMovdqu, rm.r, mem)
	default:
		panic("BUG")
	}
	m.insert(store)
}

func (m *machine) lowerCall(si *ssa.Instruction) {
	isDirectCall := si.Opcode() == ssa.OpcodeCall
	var indirectCalleePtr ssa.Value
	var directCallee ssa.FuncRef
	var sigID ssa.SignatureID
	var args []ssa.Value
	if isDirectCall {
		directCallee, sigID, args = si.CallData()
	} else {
		indirectCalleePtr, sigID, args = si.CallIndirectData()
	}
	calleeABI := m.c.GetFunctionABI(m.c.SSABuilder().ResolveSignature(sigID))

	stackSlotSize := calleeABI.AlignedArgResultStackSlotSize()
	if m.maxRequiredStackSizeForCalls < stackSlotSize+16 {
		m.maxRequiredStackSizeForCalls = stackSlotSize + 16 // 16 == return address + RBP.
	}

	// Note: See machine.SetupPrologue for the stack layout.
	// The stack pointer decrease/increase will be inserted later in the compilation.

	for i, arg := range args {
		reg := m.c.VRegOf(arg)
		def := m.c.ValueDefinition(arg)
		m.callerGenVRegToFunctionArg(calleeABI, i, reg, def, stackSlotSize)
	}

	if isDirectCall {
		call := m.allocateInstr().asCall(directCallee, calleeABI)
		m.insert(call)
	} else {
		ptrOp := m.getOperand_Mem_Reg(m.c.ValueDefinition(indirectCalleePtr))
		callInd := m.allocateInstr().asCallIndirect(ptrOp, calleeABI)
		m.insert(callInd)
	}

	var index int
	r1, rs := si.Returns()
	if r1.Valid() {
		m.callerGenFunctionReturnVReg(calleeABI, 0, m.c.VRegOf(r1), stackSlotSize)
		index++
	}

	for _, r := range rs {
		m.callerGenFunctionReturnVReg(calleeABI, index, m.c.VRegOf(r), stackSlotSize)
		index++
	}
}

// callerGenVRegToFunctionArg is the opposite of GenFunctionArgToVReg, which is used to generate the
// caller side of the function call.
func (m *machine) callerGenVRegToFunctionArg(a *backend.FunctionABI, argIndex int, reg regalloc.VReg, def *backend.SSAValueDefinition, stackSlotSize int64) {
	arg := &a.Args[argIndex]
	if def != nil && def.IsFromInstr() {
		// Constant instructions are inlined.
		if inst := def.Instr; inst.Constant() {
			m.InsertLoadConstant(inst, reg)
		}
	}
	if arg.Kind == backend.ABIArgKindReg {
		m.InsertMove(arg.Reg, reg, arg.Type)
	} else {
		store := m.allocateInstr()
		mem := newOperandMem(newAmodeImmReg(
			// -stackSlotSize because the stack pointer is not yet decreased.
			uint32(arg.Offset-stackSlotSize), rspVReg))
		switch arg.Type {
		case ssa.TypeI32:
			store.asMovRM(reg, mem, 4)
		case ssa.TypeI64:
			store.asMovRM(reg, mem, 8)
		case ssa.TypeF32:
			store.asXmmMovRM(sseOpcodeMovss, reg, mem)
		case ssa.TypeF64:
			store.asXmmMovRM(sseOpcodeMovsd, reg, mem)
		case ssa.TypeV128:
			store.asXmmMovRM(sseOpcodeMovdqu, reg, mem)
		default:
			panic("BUG")
		}
		m.insert(store)
	}
}

func (m *machine) callerGenFunctionReturnVReg(a *backend.FunctionABI, retIndex int, reg regalloc.VReg, stackSlotSize int64) {
	r := &a.Rets[retIndex]
	if r.Kind == backend.ABIArgKindReg {
		m.InsertMove(reg, r.Reg, r.Type)
	} else {
		load := m.allocateInstr()
		mem := newOperandMem(newAmodeImmReg(
			// -stackSlotSize because the stack pointer is not yet decreased.
			uint32(a.ArgStackSize+r.Offset-stackSlotSize), rspVReg))
		switch r.Type {
		case ssa.TypeI32:
			load.asMovzxRmR(extModeLQ, mem, reg)
		case ssa.TypeI64:
			load.asMov64MR(mem, reg)
		case ssa.TypeF32:
			load.asXmmUnaryRmR(sseOpcodeMovss, mem, reg)
		case ssa.TypeF64:
			load.asXmmUnaryRmR(sseOpcodeMovsd, mem, reg)
		case ssa.TypeV128:
			load.asXmmUnaryRmR(sseOpcodeMovdqu, mem, reg)
		default:
			panic("BUG")
		}
		m.insert(load)
	}
}

// InsertMove implements backend.Machine.
func (m *machine) InsertMove(dst, src regalloc.VReg, typ ssa.Type) {
	switch typ {
	case ssa.TypeI32, ssa.TypeI64:
		i := m.allocateInstr().asMovRR(src, dst, typ.Bits() == 64)
		m.insert(i)
	case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
		var op sseOpcode
		switch typ {
		case ssa.TypeF32:
			op = sseOpcodeMovss
		case ssa.TypeF64:
			op = sseOpcodeMovsd
		case ssa.TypeV128:
			op = sseOpcodeMovdqa
		}
		i := m.allocateInstr().asXmmUnaryRmR(op, newOperandReg(src), dst)
		m.insert(i)
	default:
		panic("BUG")
	}
}

// Format implements backend.Machine.
func (m *machine) Format() string {
	ectx := m.ectx
	begins := map[*instruction]backend.Label{}
	for l, pos := range ectx.LabelPositions {
		begins[pos.Begin] = l
	}

	irBlocks := map[backend.Label]ssa.BasicBlockID{}
	for i, l := range ectx.SsaBlockIDToLabels {
		irBlocks[l] = ssa.BasicBlockID(i)
	}

	var lines []string
	for cur := ectx.RootInstr; cur != nil; cur = cur.next {
		if l, ok := begins[cur]; ok {
			var labelStr string
			if blkID, ok := irBlocks[l]; ok {
				labelStr = fmt.Sprintf("%s (SSA Block: %s):", l, blkID)
			} else {
				labelStr = fmt.Sprintf("%s:", l)
			}
			lines = append(lines, labelStr)
		}
		if cur.kind == nop0 {
			continue
		}
		lines = append(lines, "\t"+cur.String())
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func (m *machine) encodeWithoutSSA(root *instruction) {
	m.labelResolutionPends = m.labelResolutionPends[:0]
	ectx := m.ectx

	bufPtr := m.c.BufPtr()
	for cur := root; cur != nil; cur = cur.next {
		offset := int64(len(*bufPtr))
		if cur.kind == nop0 {
			l := cur.nop0Label()
			if pos, ok := ectx.LabelPositions[l]; ok {
				pos.BinaryOffset = offset
			}
		}

		needLabelResolution := cur.encode(m.c)
		if needLabelResolution {
			m.labelResolutionPends = append(m.labelResolutionPends,
				labelResolutionPend{instr: cur, imm32Offset: int64(len(*bufPtr)) - 4},
			)
		}
	}

	for i := range m.labelResolutionPends {
		p := &m.labelResolutionPends[i]
		switch p.instr.kind {
		case jmp, jmpIf, lea:
			target := p.instr.jmpLabel()
			targetOffset := ectx.LabelPositions[target].BinaryOffset
			imm32Offset := p.imm32Offset
			jmpOffset := int32(targetOffset - (p.imm32Offset + 4)) // +4 because RIP points to the next instruction.
			binary.LittleEndian.PutUint32((*bufPtr)[imm32Offset:], uint32(jmpOffset))
		default:
			panic("BUG")
		}
	}
}

// Encode implements backend.Machine Encode.
func (m *machine) Encode(context.Context) {
	ectx := m.ectx
	bufPtr := m.c.BufPtr()

	m.labelResolutionPends = m.labelResolutionPends[:0]
	for _, pos := range ectx.OrderedBlockLabels {
		offset := int64(len(*bufPtr))
		pos.BinaryOffset = offset
		for cur := pos.Begin; cur != pos.End.next; cur = cur.next {
			offset := int64(len(*bufPtr))
			if cur.kind == nop0 {
				l := cur.nop0Label()
				if pos, ok := ectx.LabelPositions[l]; ok {
					pos.BinaryOffset = offset
				}
			}

			needLabelResolution := cur.encode(m.c)
			if needLabelResolution {
				m.labelResolutionPends = append(m.labelResolutionPends,
					labelResolutionPend{instr: cur, instrOffset: offset, imm32Offset: int64(len(*bufPtr)) - 4},
				)
			}
		}
	}

	buf := *bufPtr
	for i := range m.labelResolutionPends {
		p := &m.labelResolutionPends[i]
		switch p.instr.kind {
		case jmp, jmpIf, lea:
			target := p.instr.jmpLabel()
			targetOffset := ectx.LabelPositions[target].BinaryOffset
			imm32Offset := p.imm32Offset
			jmpOffset := int32(targetOffset - (p.imm32Offset + 4)) // +4 because RIP points to the next instruction.
			binary.LittleEndian.PutUint32(buf[imm32Offset:], uint32(jmpOffset))
		case jmpTableIsland:
			tableBegin := p.instrOffset
			// Each entry is the offset from the beginning of the jmpTableIsland instruction in 8 bytes.
			for i, l := range p.instr.targets {
				targetOffset := ectx.LabelPositions[backend.Label(l)].BinaryOffset
				jmpOffset := targetOffset - tableBegin
				binary.LittleEndian.PutUint64(buf[tableBegin+int64(i)*8:], uint64(jmpOffset))
			}
		default:
			panic("BUG")
		}
	}
}

// ResolveRelocations implements backend.Machine.
func (m *machine) ResolveRelocations(refToBinaryOffset map[ssa.FuncRef]int, binary []byte, relocations []backend.RelocationInfo) {
	for _, r := range relocations {
		offset := r.Offset
		calleeFnOffset := refToBinaryOffset[r.FuncRef]
		// offset is the offset of the last 4 bytes of the call instruction.
		callInstrOffsetBytes := binary[offset : offset+4]
		diff := int64(calleeFnOffset) - (offset + 4) // +4 because we want the offset of the next instruction (In x64, RIP always points to the next instruction).
		callInstrOffsetBytes[0] = byte(diff)
		callInstrOffsetBytes[1] = byte(diff >> 8)
		callInstrOffsetBytes[2] = byte(diff >> 16)
		callInstrOffsetBytes[3] = byte(diff >> 24)
	}
}

func (m *machine) lowerIcmpToFlag(xd, yd *backend.SSAValueDefinition, _64 bool) {
	x := m.getOperand_Reg(xd)
	y := m.getOperand_Mem_Imm32_Reg(yd)
	cmp := m.allocateInstr().asCmpRmiR(true, y, x.r, _64)
	m.insert(cmp)
}

func (m *machine) lowerFcmpToFlags(instr *ssa.Instruction) (f1, f2 cond, and bool) {
	x, y, c := instr.FcmpData()
	switch c {
	case ssa.FloatCmpCondEqual:
		f1, f2 = condNP, condZ
		and = true
	case ssa.FloatCmpCondNotEqual:
		f1, f2 = condP, condNZ
	case ssa.FloatCmpCondLessThan:
		f1 = condFromSSAFloatCmpCond(ssa.FloatCmpCondGreaterThan)
		f2 = condInvalid
		x, y = y, x
	case ssa.FloatCmpCondLessThanOrEqual:
		f1 = condFromSSAFloatCmpCond(ssa.FloatCmpCondGreaterThanOrEqual)
		f2 = condInvalid
		x, y = y, x
	default:
		f1 = condFromSSAFloatCmpCond(c)
		f2 = condInvalid
	}

	var opc sseOpcode
	if x.Type() == ssa.TypeF32 {
		opc = sseOpcodeUcomiss
	} else {
		opc = sseOpcodeUcomisd
	}

	xr := m.getOperand_Reg(m.c.ValueDefinition(x))
	yr := m.getOperand_Mem_Reg(m.c.ValueDefinition(y))
	m.insert(m.allocateInstr().asXmmCmpRmR(opc, yr, xr.r))
	return
}

// allocateInstr allocates an instruction.
func (m *machine) allocateInstr() *instruction {
	instr := m.ectx.InstructionPool.Allocate()
	if !m.regAllocStarted {
		instr.addedBeforeRegAlloc = true
	}
	return instr
}

func (m *machine) allocateNop() *instruction {
	instr := m.allocateInstr()
	instr.kind = nop0
	return instr
}

func (m *machine) insert(i *instruction) {
	ectx := m.ectx
	ectx.PendingInstructions = append(ectx.PendingInstructions, i)
}

func (m *machine) allocateBrTarget() (nop *instruction, l backend.Label) { //nolint
	ectx := m.ectx
	l = ectx.AllocateLabel()
	nop = m.allocateInstr()
	nop.asNop0WithLabel(l)
	pos := ectx.AllocateLabelPosition(l)
	pos.Begin, pos.End = nop, nop
	ectx.LabelPositions[l] = pos
	return
}

func (m *machine) getVRegSpillSlotOffsetFromSP(id regalloc.VRegID, size byte) int64 {
	offset, ok := m.spillSlots[id]
	if !ok {
		offset = m.spillSlotSize
		m.spillSlots[id] = offset
		m.spillSlotSize += int64(size)
	}
	return offset
}

func (m *machine) copyTo(src regalloc.VReg, dst regalloc.VReg) {
	typ := m.c.TypeOf(src)
	mov := m.allocateInstr()
	if typ.IsInt() {
		mov.asMovRR(src, dst, true)
	} else {
		mov.asXmmUnaryRmR(sseOpcodeMovdqu, newOperandReg(src), dst)
	}
	m.insert(mov)
}

func (m *machine) copyToTmp(v regalloc.VReg) regalloc.VReg {
	typ := m.c.TypeOf(v)
	tmp := m.c.AllocateVReg(typ)
	m.copyTo(v, tmp)
	return tmp
}

func (m *machine) requiredStackSize() int64 {
	return m.maxRequiredStackSizeForCalls +
		m.frameSize() +
		16 + // Need for stack checking.
		16 // return address and the caller RBP.
}

func (m *machine) frameSize() int64 {
	s := m.clobberedRegSlotSize() + m.spillSlotSize
	if s&0xf != 0 {
		panic(fmt.Errorf("BUG: frame size %d is not 16-byte aligned", s))
	}
	return s
}

func (m *machine) clobberedRegSlotSize() int64 {
	return int64(len(m.clobberedRegs) * 16)
}

func (m *machine) lowerIDivRem(si *ssa.Instruction, isDiv bool, signed bool) {
	x, y, execCtx := si.Arg3()
	if !x.Type().IsInt() {
		panic("BUG?")
	}
	_64 := x.Type().Bits() == 64

	xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
	xr := m.getOperand_Reg(xDef)
	yr := m.getOperand_Reg(yDef)
	ctxVReg := m.c.VRegOf(execCtx)
	rd := m.c.VRegOf(si.Return())

	// Ensure yr is not zero.
	test := m.allocateInstr()
	test.asCmpRmiR(false, yr, yr.r, _64)
	m.insert(test)

	// We need to copy the execution context to a temp register *BEFORE BRANCHING*, because if it's spilled,
	// it might end up being reloaded inside the exiting branch.
	execCtxTmp := m.copyToTmp(ctxVReg)

	jnz := m.allocateInstr()
	m.insert(jnz)

	nz := m.lowerExitWithCode(execCtxTmp, wazevoapi.ExitCodeIntegerDivisionByZero)

	// If not zero, we can proceed with the division.
	jnz.asJmpIf(condNZ, newOperandLabel(nz))

	m.copyTo(xr.r, raxVReg)

	var ifRemNeg1 *instruction
	if signed {
		var neg1 uint64
		if _64 {
			neg1 = 0xffffffffffffffff
		} else {
			neg1 = 0xffffffff
		}
		tmp1 := m.c.AllocateVReg(si.Return().Type())
		m.lowerIconst(tmp1, neg1, _64)

		if isDiv {
			// For signed division, we have to have branches for "math.MinInt{32,64} / -1"
			// case which results in the floating point exception via division error as
			// the resulting value exceeds the maximum of signed int.

			// First, we check if the divisor is -1.
			cmp := m.allocateInstr()
			cmp.asCmpRmiR(true, newOperandReg(tmp1), yr.r, _64)
			m.insert(cmp)

			// Again, we need to copy the execution context to a temp register *BEFORE BRANCHING*, because if it's spilled,
			// it might end up being reloaded inside the exiting branch.
			execCtxTmp2 := m.copyToTmp(execCtxTmp)
			ifNotNeg1 := m.allocateInstr()
			m.insert(ifNotNeg1)

			var minInt uint64
			if _64 {
				minInt = 0x8000000000000000
			} else {
				minInt = 0x80000000
			}
			tmp := m.c.AllocateVReg(si.Return().Type())
			m.lowerIconst(tmp, minInt, _64)

			// Next we check if the quotient is the most negative value for the signed integer, i.e.
			// if we are trying to do (math.MinInt32 / -1) or (math.MinInt64 / -1) respectively.
			cmp2 := m.allocateInstr()
			cmp2.asCmpRmiR(true, newOperandReg(tmp), xr.r, _64)
			m.insert(cmp2)

			ifNotMinInt := m.allocateInstr()
			m.insert(ifNotMinInt)

			// Trap if we are trying to do (math.MinInt32 / -1) or (math.MinInt64 / -1),
			// as that is the overflow in division as the result becomes 2^31 which is larger than
			// the maximum of signed 32-bit int (2^31-1).
			end := m.lowerExitWithCode(execCtxTmp2, wazevoapi.ExitCodeIntegerOverflow)
			ifNotNeg1.asJmpIf(condNZ, newOperandLabel(end))
			ifNotMinInt.asJmpIf(condNZ, newOperandLabel(end))
		} else {
			// If it is remainder, zeros DX register and compare the divisor to -1.
			xor := m.allocateInstr()
			xor.asAluRmiR(aluRmiROpcodeXor, newOperandReg(rdxVReg), rdxVReg, _64)
			m.insert(xor)

			// We check if the divisor is -1.
			cmp := m.allocateInstr()
			cmp.asCmpRmiR(true, newOperandReg(tmp1), yr.r, _64)
			m.insert(cmp)

			ifRemNeg1 = m.allocateInstr()
			m.insert(ifRemNeg1)
		}

		// Sign-extend DX register to have 2*x.Type().Bits() dividend over DX and AX registers.
		sed := m.allocateInstr()
		sed.asSignExtendData(_64)
		m.insert(sed)
	} else {
		// Zeros DX register to have 2*x.Type().Bits() dividend over DX and AX registers.
		xor := m.allocateInstr()
		xor.asAluRmiR(aluRmiROpcodeXor, newOperandReg(rdxVReg), rdxVReg, _64)
		m.insert(xor)
	}

	div := m.allocateInstr()
	div.asDiv(yr, signed, _64)
	m.insert(div)

	nop, end := m.allocateBrTarget()
	m.insert(nop)
	// If we are compiling a Rem instruction, when the divisor is -1 we land at the end of the function.
	if ifRemNeg1 != nil {
		ifRemNeg1.asJmpIf(condZ, newOperandLabel(end))
	}

	if isDiv {
		m.copyTo(raxVReg, rd)
	} else {
		m.copyTo(rdxVReg, rd)
	}
}

func (m *machine) lowerRound(instr *ssa.Instruction, imm roundingMode) {
	x := instr.Arg()
	if !x.Type().IsFloat() {
		panic("BUG?")
	}
	var op sseOpcode
	if x.Type().Bits() == 64 {
		op = sseOpcodeRoundsd
	} else {
		op = sseOpcodeRoundss
	}

	xDef := m.c.ValueDefinition(x)
	rm := m.getOperand_Mem_Reg(xDef)
	rd := m.c.VRegOf(instr.Return())

	xmm := m.allocateInstr().asXmmUnaryRmRImm(op, uint8(imm), rm, rd)
	m.insert(xmm)
}

func (m *machine) lowerFminFmax(instr *ssa.Instruction) {
	x, y := instr.Arg2()
	if !x.Type().IsFloat() {
		panic("BUG?")
	}

	_64 := x.Type().Bits() == 64
	isMin := instr.Opcode() == ssa.OpcodeFmin
	var minMaxOp sseOpcode

	switch {
	case _64 && isMin:
		minMaxOp = sseOpcodeMinpd
	case _64 && !isMin:
		minMaxOp = sseOpcodeMaxpd
	case !_64 && isMin:
		minMaxOp = sseOpcodeMinps
	case !_64 && !isMin:
		minMaxOp = sseOpcodeMaxps
	}

	xDef, yDef := m.c.ValueDefinition(x), m.c.ValueDefinition(y)
	rm := m.getOperand_Reg(xDef)
	rn := m.getOperand_Mem_Reg(yDef)
	rd := m.c.VRegOf(instr.Return())

	tmp := m.copyToTmp(rm.r)

	// Check if this is (either x1 or x2 is NaN) or (x1 equals x2) case.
	cmp := m.allocateInstr()
	if _64 {
		cmp.asXmmCmpRmR(sseOpcodeUcomisd, rn, tmp)
	} else {
		cmp.asXmmCmpRmR(sseOpcodeUcomiss, rn, tmp)
	}
	m.insert(cmp)

	// At this point, we have the three cases of conditional flags below
	// (See https://www.felixcloutier.com/x86/ucomiss#operation for detail.)
	//
	// 1) Two values are NaN-free and different: All flags are cleared.
	// 2) Two values are NaN-free and equal: Only ZF flags is set.
	// 3) One of Two values is NaN: ZF, PF and CF flags are set.

	// Jump instruction to handle 1) case by checking the ZF flag
	// as ZF is only set for 2) and 3) cases.
	nanFreeOrDiffJump := m.allocateInstr()
	m.insert(nanFreeOrDiffJump)

	// Start handling 2) and 3).

	// Jump if one of two values is NaN by checking the parity flag (PF).
	ifIsNan := m.allocateInstr()
	m.insert(ifIsNan)

	// Start handling 2) NaN-free and equal.

	// Before we exit this case, we have to ensure that positive zero (or negative zero for min instruction) is
	// returned if two values are positive and negative zeros.
	var op sseOpcode
	switch {
	case !_64 && isMin:
		op = sseOpcodeOrps
	case _64 && isMin:
		op = sseOpcodeOrpd
	case !_64 && !isMin:
		op = sseOpcodeAndps
	case _64 && !isMin:
		op = sseOpcodeAndpd
	}
	orAnd := m.allocateInstr()
	orAnd.asXmmRmR(op, rn, tmp)
	m.insert(orAnd)

	// Done, jump to end.
	sameExitJump := m.allocateInstr()
	m.insert(sameExitJump)

	// Start handling 3) either is NaN.
	isNanTarget, isNan := m.allocateBrTarget()
	m.insert(isNanTarget)
	ifIsNan.asJmpIf(condP, newOperandLabel(isNan))

	// We emit the ADD instruction to produce the NaN in tmp.
	add := m.allocateInstr()
	if _64 {
		add.asXmmRmR(sseOpcodeAddsd, rn, tmp)
	} else {
		add.asXmmRmR(sseOpcodeAddss, rn, tmp)
	}
	m.insert(add)

	// Exit from the NaN case branch.
	nanExitJmp := m.allocateInstr()
	m.insert(nanExitJmp)

	// Start handling 1).
	doMinMaxTarget, doMinMax := m.allocateBrTarget()
	m.insert(doMinMaxTarget)
	nanFreeOrDiffJump.asJmpIf(condNZ, newOperandLabel(doMinMax))

	// Now handle the NaN-free and different values case.
	minMax := m.allocateInstr()
	minMax.asXmmRmR(minMaxOp, rn, tmp)
	m.insert(minMax)

	endNop, end := m.allocateBrTarget()
	m.insert(endNop)
	nanExitJmp.asJmp(newOperandLabel(end))
	sameExitJump.asJmp(newOperandLabel(end))

	m.copyTo(tmp, rd)
}
