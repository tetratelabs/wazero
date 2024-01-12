package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

type instruction struct {
	prev, next          *instruction
	abi                 *backend.FunctionABI
	op1, op2            operand
	u1, u2              uint64
	b1                  bool
	addedBeforeRegAlloc bool
	kind                instructionKind
}

// Next implements regalloc.Instr.
func (i *instruction) Next() regalloc.Instr {
	return i.next
}

// Prev implements regalloc.Instr.
func (i *instruction) Prev() regalloc.Instr {
	return i.prev
}

// IsCall implements regalloc.Instr.
func (i *instruction) IsCall() bool { return i.kind == call }

// IsIndirectCall implements regalloc.Instr.
func (i *instruction) IsIndirectCall() bool { return i.kind == callIndirect }

// IsReturn implements regalloc.Instr.
func (i *instruction) IsReturn() bool { return i.kind == ret }

// AddedBeforeRegAlloc implements regalloc.Instr.
func (i *instruction) AddedBeforeRegAlloc() bool { return i.addedBeforeRegAlloc }

// String implements regalloc.Instr.
func (i *instruction) String() string {
	switch i.kind {
	case nop0:
		return "nop"
	case ret:
		return "ret"
	case imm:
		if i.b1 {
			return fmt.Sprintf("movabsq $%d, %s", int64(i.u1), i.op2.format(true))
		} else {
			return fmt.Sprintf("movl $%d, %s", int32(i.u1), i.op2.format(false))
		}
	case aluRmiR:
		return fmt.Sprintf("%s %s, %s", aluRmiROpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case movRR:
		if i.b1 {
			return fmt.Sprintf("movq %s, %s", i.op1.format(true), i.op2.format(true))
		} else {
			return fmt.Sprintf("movl %s, %s", i.op1.format(false), i.op2.format(false))
		}
	case xmmRmR:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case gprToXmm:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case xmmUnaryRmR:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case unaryRmR:
		return fmt.Sprintf("%s %s, %s", unaryRmROpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case not:
		return fmt.Sprintf("not %s", i.op1.format(i.b1))
	case neg:
		return fmt.Sprintf("neg %s", i.op1.format(i.b1))
	case div:
		panic("TODO")
	case mulHi:
		if i.u1 != 0 {
			return fmt.Sprintf("imul %s", i.op1.format(i.b1))
		} else {
			return fmt.Sprintf("mul %s", i.op1.format(i.b1))
		}
	case checkedDivOrRemSeq:
		panic("TODO")
	case signExtendData:
		panic("TODO")
	case movzxRmR:
		return fmt.Sprintf("movzx.%s %s, %s", extMode(i.u1), i.op1.format(true), i.op2.format(true))
	case mov64MR:
		return fmt.Sprintf("movq %s, %s", i.op1.format(true), i.op2.format(true))
	case lea:
		return fmt.Sprintf("lea %s, %s", i.op1.format(true), i.op2.format(true))
	case movsxRmR:
		return fmt.Sprintf("movsx.%s %s, %s", extMode(i.u1), i.op1.format(true), i.op2.format(true))
	case movRM:
		var suffix string
		switch i.u1 {
		case 1:
			suffix = "b"
		case 2:
			suffix = "w"
		case 4:
			suffix = "l"
		case 8:
			suffix = "q"
		}
		return fmt.Sprintf("mov.%s %s, %s", suffix, i.op1.format(true), i.op2.format(true))
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
		return fmt.Sprintf("pushq %s", i.op1.format(true))
	case pop64:
		return fmt.Sprintf("popq %s", i.op1.format(true))
	case xmmMovRM:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(true), i.op2.format(true))
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
	case jmp:
		return fmt.Sprintf("jmp %s", i.op1.format(true))
	case jmpIf:
		return fmt.Sprintf("j%s %s", cond(i.u1), i.op1.format(true))
	case jmpTableSeq:
		panic("TODO")
	case exitIf:
		panic("TODO")
	case ud2:
		return "ud2"
	case call:
		if i.u2 > 0 {
			return fmt.Sprintf("call $%d", int32(i.u2))
		} else {
			return fmt.Sprintf("call %s", ssa.FuncRef(i.u1))
		}
	case callIndirect:
		return fmt.Sprintf("callq *%s", i.op1.format(true))
	default:
		panic(fmt.Sprintf("BUG: %d", int(i.kind)))
	}
}

// Defs implements regalloc.Instr.
func (i *instruction) Defs(regs *[]regalloc.VReg) []regalloc.VReg {
	*regs = (*regs)[:0]
	switch dk := defKinds[i.kind]; dk {
	case defKindNone:
	case defKindOp2:
		if !i.op2.r.Valid() {
			panic("BUG" + i.String())
		}
		*regs = append(*regs, i.op2.r)
	default:
		panic(fmt.Sprintf("BUG: invalid defKind \"%s\" for %s", dk, i))
	}
	return *regs
}

// Uses implements regalloc.Instr.
func (i *instruction) Uses(regs *[]regalloc.VReg) []regalloc.VReg {
	*regs = (*regs)[:0]
	switch uk := useKinds[i.kind]; uk {
	case useKindNone:
	case useKindOp1:
		op := i.op1
		switch op.kind {
		case operandKindReg:
			*regs = append(*regs, op.r)
		case operandKindMem:
			op.amode.uses(regs)
		case operandKindImm32, operandKindLabel:
		default:
			panic(fmt.Sprintf("BUG: invalid operand: %s", i))
		}
	default:
		panic(fmt.Sprintf("BUG: invalid useKind %s for %s", uk, i))
	}
	return *regs
}

// AssignUse implements regalloc.Instr.
func (i *instruction) AssignUse(index int, v regalloc.VReg) {
	switch uk := useKinds[i.kind]; uk {
	case useKindNone:
	case useKindOp1:
		op := &i.op1
		switch op.kind {
		case operandKindReg:
			if index != 0 {
				panic("BUG")
			}
			if op.r.IsRealReg() {
				panic("BUG already assigned: " + i.String())
			}
			op.r = v
		case operandKindMem:
			op.amode.assignUses(index, v)
		default:
			panic(fmt.Sprintf("BUG: invalid operand: %s", i))
		}
	default:
		panic(fmt.Sprintf("BUG: invalid useKind %s for %s", uk, i))
	}
}

// AssignDef implements regalloc.Instr.
func (i *instruction) AssignDef(reg regalloc.VReg) {
	switch dk := defKinds[i.kind]; dk {
	case defKindNone:
	case defKindOp2:
		if !i.op2.r.Valid() {
			panic("BUG already assigned" + i.String())
		}
		i.op2.r = reg
	default:
		panic(fmt.Sprintf("BUG: invalid defKind \"%s\" for %s", dk, i))
	}
}

// IsCopy implements regalloc.Instr.
func (i *instruction) IsCopy() bool {
	k := i.kind
	return k == movRR || k == xmmUnaryRmR
}

func resetInstruction(i *instruction) {
	*i = instruction{}
}

func setNext(i *instruction, next *instruction) {
	i.next = next
}

func setPrev(i *instruction, prev *instruction) {
	i.prev = prev
}

func asNop(i *instruction) {
	i.kind = nop0
}

func (i *instruction) asNop0WithLabel(label backend.Label) *instruction { //nolint
	i.kind = nop0
	i.u1 = uint64(label)
	return i
}

func (i *instruction) nop0Label() backend.Label {
	return backend.Label(i.u1)
}

type instructionKind int

const (
	nop0 instructionKind = iota + 1

	// Integer arithmetic/bit-twiddling: (add sub and or xor mul, etc.) (32 64) (reg addr imm) reg
	aluRmiR

	// Instructions on GPR that only read src and defines dst (dst is not modified): bsr, etc.
	unaryRmR

	// Bitwise not
	not

	// Integer negation
	neg

	// Integer quotient and remainder: (div idiv) $rax $rdx (reg addr)
	div

	// The high bits (RDX) of a (un)signed multiply: RDX:RAX := RAX * rhs.
	mulHi

	// A synthetic sequence to implement the right inline checks for remainder and division,
	// assuming the dividend is in %rax.
	// Puts the result back into %rax if is_div, %rdx if !is_div, to mimic what the div
	// instruction does.
	// The generated code sequence is described in the emit's function match arm for this
	// instruction.
	///
	// Note: %rdx is marked as modified by this instruction, to avoid an early clobber problem
	// with the temporary and divisor registers. Make sure to zero %rdx right before this
	// instruction, or you might run into regalloc failures where %rdx is live before its first
	// def!
	checkedDivOrRemSeq

	// Do a sign-extend based on the sign of the value in rax into rdx: (cwd cdq cqo)
	// or al into ah: (cbw)
	signExtendData

	// Constant materialization: (imm32 imm64) reg.
	// Either: movl $imm32, %reg32 or movabsq $imm64, %reg64.
	imm

	// GPR to GPR move: mov (64 32) reg reg.
	movRR

	// movzxRmR is zero-extended loads or move (R to R), except for 64 bits: movz (bl bq wl wq lq) addr reg.
	// Note that the lq variant doesn't really exist since the default zero-extend rule makes it
	// unnecessary. For that case we emit the equivalent "movl AM, reg32".
	movzxRmR

	// mov64MR is a plain 64-bit integer load, since movzxRmR can't represent that.
	mov64MR

	// Loads the memory address of addr into dst.
	lea

	// Sign-extended loads and moves: movs (bl bq wl wq lq) addr reg.
	movsxRmR

	// Integer stores: mov (b w l q) reg addr.
	movRM

	// Arithmetic shifts: (shl shr sar) (b w l q) imm reg.
	shiftR

	// Arithmetic SIMD shifts.
	xmmRmiReg

	// Integer comparisons/tests: cmp or test (b w l q) (reg addr imm) reg.
	cmpRmiR

	// Materializes the requested condition code in the destination reg.
	setcc

	// Integer conditional move.
	// Overwrites the destination register.
	cmove

	// pushq (reg addr imm)
	push64

	// popq reg
	pop64

	// XMM (scalar or vector) binary op: (add sub and or xor mul adc? sbb?) (32 64) (reg addr) reg
	xmmRmR

	// XMM (scalar or vector) unary op: mov between XMM registers (32 64) (reg addr) reg.
	//
	// This differs from xmmRmR in that the dst register of xmmUnaryRmR is not used in the
	// computation of the instruction dst value and so does not have to be a previously valid
	// value. This is characteristic of mov instructions.
	xmmUnaryRmR

	// XMM (scalar or vector) unary op (from xmm to mem): stores, movd, movq
	xmmMovRM

	// XMM (vector) unary op (to move a constant value into an xmm register): movups
	xmmLoadConst

	// XMM (scalar) unary op (from xmm to integer reg): movd, movq, cvtts{s,d}2si
	xmmToGpr

	// XMM (scalar) unary op (from integer to float reg): movd, movq, cvtsi2s{s,d}
	gprToXmm

	// Converts an unsigned int64 to a float32/float64.
	cvtUint64ToFloatSeq

	// Converts a scalar xmm to a signed int32/int64.
	cvtFloatToSintSeq

	// Converts a scalar xmm to an unsigned int32/int64.
	cvtFloatToUintSeq

	// A sequence to compute min/max with the proper NaN semantics for xmm registers.
	xmmMinMaxSeq

	// XMM (scalar) conditional move.
	// Overwrites the destination register if cc is set.
	xmmCmove

	// Float comparisons/tests: cmp (b w l q) (reg addr imm) reg.
	xmmCmpRmR

	// A binary XMM instruction with an 8-bit immediate: e.g. cmp (ps pd) imm (reg addr) reg
	xmmRmRImm

	// Direct call: call simm32.
	// Note that the offset is the relative to the *current RIP*, which points to the first byte of the next instruction.
	call

	// Indirect call: callq (reg mem).
	callIndirect

	// Return.
	ret

	// Jump: jmp (reg, mem, imm32 or label)
	jmp

	// Jump conditionally: jcond cond label.
	jmpIf

	// Jump-table sequence, as one compound instruction (see note in lower.rs for rationale).
	// The generated code sequence is described in the emit's function match arm for this
	// instruction.
	// See comment in lowering about the temporaries signedness.
	jmpTableSeq

	// Exits the execution if the condition code is set.
	exitIf

	// An instruction that will always trigger the illegal instruction exception.
	ud2

	instrMax
)

func (k instructionKind) String() string {
	switch k {
	case nop0:
		return "nop"
	case ret:
		return "ret"
	case imm:
		return "imm"
	case aluRmiR:
		return "aluRmiR"
	case movRR:
		return "movRR"
	case xmmRmR:
		return "xmmRmR"
	case gprToXmm:
		return "gprToXmm"
	case xmmUnaryRmR:
		return "xmmUnaryRmR"
	case unaryRmR:
		return "unaryRmR"
	case not:
		return "not"
	case neg:
		return "neg"
	case div:
		return "div"
	case mulHi:
		return "mulHi"
	case checkedDivOrRemSeq:
		return "checkedDivOrRemSeq"
	case signExtendData:
		return "signExtendData"
	case movzxRmR:
		return "movzxRmR"
	case mov64MR:
		return "mov64MR"
	case lea:
		return "lea"
	case movsxRmR:
		return "movsxRmR"
	case movRM:
		return "movRM"
	case shiftR:
		return "shiftR"
	case xmmRmiReg:
		return "xmmRmiReg"
	case cmpRmiR:
		return "cmpRmiR"
	case setcc:
		return "setcc"
	case cmove:
		return "cmove"
	case push64:
		return "push64"
	case pop64:
		return "pop64"
	case xmmMovRM:
		return "xmmMovRM"
	case xmmLoadConst:
		return "xmmLoadConst"
	case xmmToGpr:
		return "xmmToGpr"
	case cvtUint64ToFloatSeq:
		return "cvtUint64ToFloatSeq"
	case cvtFloatToSintSeq:
		return "cvtFloatToSintSeq"
	case cvtFloatToUintSeq:
		return "cvtFloatToUintSeq"
	case xmmMinMaxSeq:
		return "xmmMinMaxSeq"
	case xmmCmove:
		return "xmmCmove"
	case xmmCmpRmR:
		return "xmmCmpRmR"
	case xmmRmRImm:
		return "xmmRmRImm"
	case jmpIf:
		return "jmpIf"
	case jmp:
		return "jmp"
	case jmpTableSeq:
		return "jmpTableSeq"
	case exitIf:
		return "exitIf"
	case ud2:
		return "ud2"
	default:
		panic("BUG")
	}
}

type aluRmiROpcode byte

const (
	aluRmiROpcodeAdd aluRmiROpcode = iota + 1
	aluRmiROpcodeSub
	aluRmiROpcodeAnd
	aluRmiROpcodeOr
	aluRmiROpcodeXor
	aluRmiROpcodeMul
)

func (a aluRmiROpcode) String() string {
	switch a {
	case aluRmiROpcodeAdd:
		return "add"
	case aluRmiROpcodeSub:
		return "sub"
	case aluRmiROpcodeAnd:
		return "and"
	case aluRmiROpcodeOr:
		return "or"
	case aluRmiROpcodeXor:
		return "xor"
	case aluRmiROpcodeMul:
		return "imul"
	default:
		panic("BUG")
	}
}

func (i *instruction) asJmpIf(cond cond, target operand) *instruction {
	i.kind = jmpIf
	i.u1 = uint64(cond)
	i.op1 = target
	return i
}

func (i *instruction) asJmp(target operand) *instruction {
	i.kind = jmp
	i.op1 = target
	return i
}

func (i *instruction) asLEA(a amode, rd regalloc.VReg) *instruction {
	i.kind = lea
	i.op1 = newOperandMem(a)
	i.op2 = newOperandReg(rd)
	return i
}

func (i *instruction) asCall(ref ssa.FuncRef, abi *backend.FunctionABI) *instruction {
	i.kind = call
	i.abi = abi
	i.u1 = uint64(ref)
	return i
}

func (i *instruction) asCallIndirect(ptr operand, abi *backend.FunctionABI) *instruction {
	if ptr.kind != operandKindReg && ptr.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = callIndirect
	i.abi = abi
	i.op1 = ptr
	return i
}

func (i *instruction) asRet(abi *backend.FunctionABI) *instruction {
	i.kind = ret
	i.abi = abi
	return i
}

func (i *instruction) asImm(dst regalloc.VReg, value uint64, _64 bool) *instruction {
	i.kind = imm
	i.op2 = newOperandReg(dst)
	i.u1 = value
	i.b1 = _64
	return i
}

func (i *instruction) asAluRmiR(op aluRmiROpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem && rm.kind != operandKindImm32 {
		panic("BUG")
	}
	i.kind = aluRmiR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asXmmRmR(op sseOpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmRmR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asGprToXmm(op sseOpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = gprToXmm
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asMovRM(rm regalloc.VReg, rd operand, size byte) *instruction {
	if rd.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = movRM
	i.op1 = newOperandReg(rm)
	i.op2 = rd
	i.u1 = uint64(size)
	return i
}

func (i *instruction) asMovsxRmR(ext extMode, src operand, rd regalloc.VReg) *instruction {
	if src.kind != operandKindReg && src.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = movsxRmR
	i.op1 = src
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(ext)
	return i
}

func (i *instruction) asMovzxRmR(ext extMode, src operand, rd regalloc.VReg) *instruction {
	if src.kind != operandKindReg && src.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = movzxRmR
	i.op1 = src
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(ext)
	return i
}

func (i *instruction) asUD2() *instruction {
	i.kind = ud2
	return i
}

func (i *instruction) asMov64MR(rm operand, rd regalloc.VReg) *instruction {
	if rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = mov64MR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	return i
}

func (i *instruction) asMovRR(rm, rd regalloc.VReg, _64 bool) *instruction {
	i.kind = movRR
	i.op1 = newOperandReg(rm)
	i.op2 = newOperandReg(rd)
	i.b1 = _64
	return i
}

func (i *instruction) asNot(rm operand, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = not
	i.op1 = rm
	i.b1 = _64
	return i
}

func (i *instruction) asNeg(rm operand, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = neg
	i.op1 = rm
	i.b1 = _64
	return i
}

func (i *instruction) asMulHi(rm operand, signed, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = mulHi
	i.op1 = rm
	i.b1 = _64
	if signed {
		i.u1 = 1
	}
	return i
}

func (i *instruction) asUnaryRmR(op unaryRmROpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = unaryRmR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asXmmUnaryRmR(op sseOpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmUnaryRmR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asXmmMovRM(op sseOpcode, rm regalloc.VReg, rd operand) *instruction {
	if rd.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmMovRM
	i.op1 = newOperandReg(rm)
	i.op2 = rd
	i.u1 = uint64(op)
	return i
}

func (i *instruction) asPop64(rm regalloc.VReg) *instruction {
	i.kind = pop64
	i.op1 = newOperandReg(rm)
	return i
}

func (i *instruction) asPush64(op operand) *instruction {
	if op.kind != operandKindReg && op.kind != operandKindMem && op.kind != operandKindImm32 {
		panic("BUG")
	}
	i.kind = push64
	i.op1 = op
	return i
}

type unaryRmROpcode byte

const (
	unaryRmROpcodeBsr unaryRmROpcode = iota
	unaryRmROpcodeBsf
	unaryRmROpcodeLzcnt
	unaryRmROpcodeTzcnt
	unaryRmROpcodePopcnt
)

func (u unaryRmROpcode) String() string {
	switch u {
	case unaryRmROpcodeBsr:
		return "bsr"
	case unaryRmROpcodeBsf:
		return "bsf"
	case unaryRmROpcodeLzcnt:
		return "lzcnt"
	case unaryRmROpcodeTzcnt:
		return "tzcnt"
	case unaryRmROpcodePopcnt:
		return "popcnt"
	default:
		panic("BUG")
	}
}

type sseOpcode byte

const (
	sseOpcodeAddps sseOpcode = iota + 1
	sseOpcodeAddpd
	sseOpcodeAddss
	sseOpcodeAddsd
	sseOpcodeAndps
	sseOpcodeAndpd
	sseOpcodeAndnps
	sseOpcodeAndnpd
	sseOpcodeComiss
	sseOpcodeComisd
	sseOpcodeCmpps
	sseOpcodeCmppd
	sseOpcodeCmpss
	sseOpcodeCmpsd
	sseOpcodeCvtdq2ps
	sseOpcodeCvtsd2ss
	sseOpcodeCvtsd2si
	sseOpcodeCvtsi2ss
	sseOpcodeCvtsi2sd
	sseOpcodeCvtss2si
	sseOpcodeCvtss2sd
	sseOpcodeCvttps2dq
	sseOpcodeCvttss2si
	sseOpcodeCvttsd2si
	sseOpcodeDivps
	sseOpcodeDivpd
	sseOpcodeDivss
	sseOpcodeDivsd
	sseOpcodeInsertps
	sseOpcodeMaxps
	sseOpcodeMaxpd
	sseOpcodeMaxss
	sseOpcodeMaxsd
	sseOpcodeMinps
	sseOpcodeMinpd
	sseOpcodeMinss
	sseOpcodeMinsd
	sseOpcodeMovaps
	sseOpcodeMovapd
	sseOpcodeMovd
	sseOpcodeMovdqa
	sseOpcodeMovdqu
	sseOpcodeMovlhps
	sseOpcodeMovmskps
	sseOpcodeMovmskpd
	sseOpcodeMovq
	sseOpcodeMovss
	sseOpcodeMovsd
	sseOpcodeMovups
	sseOpcodeMovupd
	sseOpcodeMulps
	sseOpcodeMulpd
	sseOpcodeMulss
	sseOpcodeMulsd
	sseOpcodeOrps
	sseOpcodeOrpd
	sseOpcodePabsb
	sseOpcodePabsw
	sseOpcodePabsd
	sseOpcodePackssdw
	sseOpcodePacksswb
	sseOpcodePackusdw
	sseOpcodePackuswb
	sseOpcodePaddb
	sseOpcodePaddd
	sseOpcodePaddq
	sseOpcodePaddw
	sseOpcodePaddsb
	sseOpcodePaddsw
	sseOpcodePaddusb
	sseOpcodePaddusw
	sseOpcodePalignr
	sseOpcodePand
	sseOpcodePandn
	sseOpcodePavgb
	sseOpcodePavgw
	sseOpcodePcmpeqb
	sseOpcodePcmpeqw
	sseOpcodePcmpeqd
	sseOpcodePcmpeqq
	sseOpcodePcmpgtb
	sseOpcodePcmpgtw
	sseOpcodePcmpgtd
	sseOpcodePcmpgtq
	sseOpcodePextrb
	sseOpcodePextrw
	sseOpcodePextrd
	sseOpcodePinsrb
	sseOpcodePinsrw
	sseOpcodePinsrd
	sseOpcodePmaddwd
	sseOpcodePmaxsb
	sseOpcodePmaxsw
	sseOpcodePmaxsd
	sseOpcodePmaxub
	sseOpcodePmaxuw
	sseOpcodePmaxud
	sseOpcodePminsb
	sseOpcodePminsw
	sseOpcodePminsd
	sseOpcodePminub
	sseOpcodePminuw
	sseOpcodePminud
	sseOpcodePmovmskb
	sseOpcodePmovsxbd
	sseOpcodePmovsxbw
	sseOpcodePmovsxbq
	sseOpcodePmovsxwd
	sseOpcodePmovsxwq
	sseOpcodePmovsxdq
	sseOpcodePmovzxbd
	sseOpcodePmovzxbw
	sseOpcodePmovzxbq
	sseOpcodePmovzxwd
	sseOpcodePmovzxwq
	sseOpcodePmovzxdq
	sseOpcodePmulld
	sseOpcodePmullw
	sseOpcodePmuludq
	sseOpcodePor
	sseOpcodePshufb
	sseOpcodePshufd
	sseOpcodePsllw
	sseOpcodePslld
	sseOpcodePsllq
	sseOpcodePsraw
	sseOpcodePsrad
	sseOpcodePsrlw
	sseOpcodePsrld
	sseOpcodePsrlq
	sseOpcodePsubb
	sseOpcodePsubd
	sseOpcodePsubq
	sseOpcodePsubw
	sseOpcodePsubsb
	sseOpcodePsubsw
	sseOpcodePsubusb
	sseOpcodePsubusw
	sseOpcodePtest
	sseOpcodePunpckhbw
	sseOpcodePunpcklbw
	sseOpcodePxor
	sseOpcodeRcpss
	sseOpcodeRoundps
	sseOpcodeRoundpd
	sseOpcodeRoundss
	sseOpcodeRoundsd
	sseOpcodeRsqrtss
	sseOpcodeSqrtps
	sseOpcodeSqrtpd
	sseOpcodeSqrtss
	sseOpcodeSqrtsd
	sseOpcodeSubps
	sseOpcodeSubpd
	sseOpcodeSubss
	sseOpcodeSubsd
	sseOpcodeUcomiss
	sseOpcodeUcomisd
	sseOpcodeXorps
	sseOpcodeXorpd
)

func (s sseOpcode) String() string {
	switch s {
	case sseOpcodeAddps:
		return "addps"
	case sseOpcodeAddpd:
		return "addpd"
	case sseOpcodeAddss:
		return "addss"
	case sseOpcodeAddsd:
		return "addsd"
	case sseOpcodeAndps:
		return "andps"
	case sseOpcodeAndpd:
		return "andpd"
	case sseOpcodeAndnps:
		return "andnps"
	case sseOpcodeAndnpd:
		return "andnpd"
	case sseOpcodeComiss:
		return "comiss"
	case sseOpcodeComisd:
		return "comisd"
	case sseOpcodeCmpps:
		return "cmpps"
	case sseOpcodeCmppd:
		return "cmppd"
	case sseOpcodeCmpss:
		return "cmpss"
	case sseOpcodeCmpsd:
		return "cmpsd"
	case sseOpcodeCvtdq2ps:
		return "cvtdq2ps"
	case sseOpcodeCvtsd2ss:
		return "cvtsd2ss"
	case sseOpcodeCvtsd2si:
		return "cvtsd2si"
	case sseOpcodeCvtsi2ss:
		return "cvtsi2ss"
	case sseOpcodeCvtsi2sd:
		return "cvtsi2sd"
	case sseOpcodeCvtss2si:
		return "cvtss2si"
	case sseOpcodeCvtss2sd:
		return "cvtss2sd"
	case sseOpcodeCvttps2dq:
		return "cvttps2dq"
	case sseOpcodeCvttss2si:
		return "cvttss2si"
	case sseOpcodeCvttsd2si:
		return "cvttsd2si"
	case sseOpcodeDivps:
		return "divps"
	case sseOpcodeDivpd:
		return "divpd"
	case sseOpcodeDivss:
		return "divss"
	case sseOpcodeDivsd:
		return "divsd"
	case sseOpcodeInsertps:
		return "insertps"
	case sseOpcodeMaxps:
		return "maxps"
	case sseOpcodeMaxpd:
		return "maxpd"
	case sseOpcodeMaxss:
		return "maxss"
	case sseOpcodeMaxsd:
		return "maxsd"
	case sseOpcodeMinps:
		return "minps"
	case sseOpcodeMinpd:
		return "minpd"
	case sseOpcodeMinss:
		return "minss"
	case sseOpcodeMinsd:
		return "minsd"
	case sseOpcodeMovaps:
		return "movaps"
	case sseOpcodeMovapd:
		return "movapd"
	case sseOpcodeMovd:
		return "movd"
	case sseOpcodeMovdqa:
		return "movdqa"
	case sseOpcodeMovdqu:
		return "movdqu"
	case sseOpcodeMovlhps:
		return "movlhps"
	case sseOpcodeMovmskps:
		return "movmskps"
	case sseOpcodeMovmskpd:
		return "movmskpd"
	case sseOpcodeMovq:
		return "movq"
	case sseOpcodeMovss:
		return "movss"
	case sseOpcodeMovsd:
		return "movsd"
	case sseOpcodeMovups:
		return "movups"
	case sseOpcodeMovupd:
		return "movupd"
	case sseOpcodeMulps:
		return "mulps"
	case sseOpcodeMulpd:
		return "mulpd"
	case sseOpcodeMulss:
		return "mulss"
	case sseOpcodeMulsd:
		return "mulsd"
	case sseOpcodeOrps:
		return "orps"
	case sseOpcodeOrpd:
		return "orpd"
	case sseOpcodePabsb:
		return "pabsb"
	case sseOpcodePabsw:
		return "pabsw"
	case sseOpcodePabsd:
		return "pabsd"
	case sseOpcodePackssdw:
		return "packssdw"
	case sseOpcodePacksswb:
		return "packsswb"
	case sseOpcodePackusdw:
		return "packusdw"
	case sseOpcodePackuswb:
		return "packuswb"
	case sseOpcodePaddb:
		return "paddb"
	case sseOpcodePaddd:
		return "paddd"
	case sseOpcodePaddq:
		return "paddq"
	case sseOpcodePaddw:
		return "paddw"
	case sseOpcodePaddsb:
		return "paddsb"
	case sseOpcodePaddsw:
		return "paddsw"
	case sseOpcodePaddusb:
		return "paddusb"
	case sseOpcodePaddusw:
		return "paddusw"
	case sseOpcodePalignr:
		return "palignr"
	case sseOpcodePand:
		return "pand"
	case sseOpcodePandn:
		return "pandn"
	case sseOpcodePavgb:
		return "pavgb"
	case sseOpcodePavgw:
		return "pavgw"
	case sseOpcodePcmpeqb:
		return "pcmpeqb"
	case sseOpcodePcmpeqw:
		return "pcmpeqw"
	case sseOpcodePcmpeqd:
		return "pcmpeqd"
	case sseOpcodePcmpeqq:
		return "pcmpeqq"
	case sseOpcodePcmpgtb:
		return "pcmpgtb"
	case sseOpcodePcmpgtw:
		return "pcmpgtw"
	case sseOpcodePcmpgtd:
		return "pcmpgtd"
	case sseOpcodePcmpgtq:
		return "pcmpgtq"
	case sseOpcodePextrb:
		return "pextrb"
	case sseOpcodePextrw:
		return "pextrw"
	case sseOpcodePextrd:
		return "pextrd"
	case sseOpcodePinsrb:
		return "pinsrb"
	case sseOpcodePinsrw:
		return "pinsrw"
	case sseOpcodePinsrd:
		return "pinsrd"
	case sseOpcodePmaddwd:
		return "pmaddwd"
	case sseOpcodePmaxsb:
		return "pmaxsb"
	case sseOpcodePmaxsw:
		return "pmaxsw"
	case sseOpcodePmaxsd:
		return "pmaxsd"
	case sseOpcodePmaxub:
		return "pmaxub"
	case sseOpcodePmaxuw:
		return "pmaxuw"
	case sseOpcodePmaxud:
		return "pmaxud"
	case sseOpcodePminsb:
		return "pminsb"
	case sseOpcodePminsw:
		return "pminsw"
	case sseOpcodePminsd:
		return "pminsd"
	case sseOpcodePminub:
		return "pminub"
	case sseOpcodePminuw:
		return "pminuw"
	case sseOpcodePminud:
		return "pminud"
	case sseOpcodePmovmskb:
		return "pmovmskb"
	case sseOpcodePmovsxbd:
		return "pmovsxbd"
	case sseOpcodePmovsxbw:
		return "pmovsxbw"
	case sseOpcodePmovsxbq:
		return "pmovsxbq"
	case sseOpcodePmovsxwd:
		return "pmovsxwd"
	case sseOpcodePmovsxwq:
		return "pmovsxwq"
	case sseOpcodePmovsxdq:
		return "pmovsxdq"
	case sseOpcodePmovzxbd:
		return "pmovzxbd"
	case sseOpcodePmovzxbw:
		return "pmovzxbw"
	case sseOpcodePmovzxbq:
		return "pmovzxbq"
	case sseOpcodePmovzxwd:
		return "pmovzxwd"
	case sseOpcodePmovzxwq:
		return "pmovzxwq"
	case sseOpcodePmovzxdq:
		return "pmovzxdq"
	case sseOpcodePmulld:
		return "pmulld"
	case sseOpcodePmullw:
		return "pmullw"
	case sseOpcodePmuludq:
		return "pmuludq"
	case sseOpcodePor:
		return "por"
	case sseOpcodePshufb:
		return "pshufb"
	case sseOpcodePshufd:
		return "pshufd"
	case sseOpcodePsllw:
		return "psllw"
	case sseOpcodePslld:
		return "pslld"
	case sseOpcodePsllq:
		return "psllq"
	case sseOpcodePsraw:
		return "psraw"
	case sseOpcodePsrad:
		return "psrad"
	case sseOpcodePsrlw:
		return "psrlw"
	case sseOpcodePsrld:
		return "psrld"
	case sseOpcodePsrlq:
		return "psrlq"
	case sseOpcodePsubb:
		return "psubb"
	case sseOpcodePsubd:
		return "psubd"
	case sseOpcodePsubq:
		return "psubq"
	case sseOpcodePsubw:
		return "psubw"
	case sseOpcodePsubsb:
		return "psubsb"
	case sseOpcodePsubsw:
		return "psubsw"
	case sseOpcodePsubusb:
		return "psubusb"
	case sseOpcodePsubusw:
		return "psubusw"
	case sseOpcodePtest:
		return "ptest"
	case sseOpcodePunpckhbw:
		return "punpckhbw"
	case sseOpcodePunpcklbw:
		return "punpcklbw"
	case sseOpcodePxor:
		return "pxor"
	case sseOpcodeRcpss:
		return "rcpss"
	case sseOpcodeRoundps:
		return "roundps"
	case sseOpcodeRoundpd:
		return "roundpd"
	case sseOpcodeRoundss:
		return "roundss"
	case sseOpcodeRoundsd:
		return "roundsd"
	case sseOpcodeRsqrtss:
		return "rsqrtss"
	case sseOpcodeSqrtps:
		return "sqrtps"
	case sseOpcodeSqrtpd:
		return "sqrtpd"
	case sseOpcodeSqrtss:
		return "sqrtss"
	case sseOpcodeSqrtsd:
		return "sqrtsd"
	case sseOpcodeSubps:
		return "subps"
	case sseOpcodeSubpd:
		return "subpd"
	case sseOpcodeSubss:
		return "subss"
	case sseOpcodeSubsd:
		return "subsd"
	case sseOpcodeUcomiss:
		return "ucomiss"
	case sseOpcodeUcomisd:
		return "ucomisd"
	case sseOpcodeXorps:
		return "xorps"
	case sseOpcodeXorpd:
		return "xorpd"
	default:
		panic("BUG")
	}
}

func linkInstr(prev, next *instruction) *instruction {
	prev.next = next
	next.prev = prev
	return next
}

type defKind byte

const (
	defKindNone defKind = iota + 1
	defKindOp2
)

var defKinds = [instrMax]defKind{
	nop0:        defKindNone,
	ret:         defKindNone,
	movRR:       defKindOp2,
	imm:         defKindOp2,
	xmmUnaryRmR: defKindOp2,
	gprToXmm:    defKindOp2,
}

// String implements fmt.Stringer.
func (d defKind) String() string {
	switch d {
	case defKindNone:
		return "none"
	case defKindOp2:
		return "op2"
	default:
		return "invalid"
	}
}

type useKind byte

const (
	useKindNone useKind = iota + 1
	useKindOp1
)

var useKinds = [instrMax]useKind{
	nop0:        useKindNone,
	ret:         useKindNone,
	movRR:       useKindOp1,
	imm:         useKindNone,
	xmmUnaryRmR: useKindOp1,
	gprToXmm:    useKindOp1,
}

func (u useKind) String() string {
	switch u {
	case useKindNone:
		return "none"
	case useKindOp1:
		return "op1"
	default:
		return "invalid"
	}
}
