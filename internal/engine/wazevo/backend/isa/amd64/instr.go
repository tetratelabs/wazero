package amd64

import (
	"fmt"
	"strings"

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
	targets             []uint32
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
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(false), i.op2.format(false))
	case gprToXmm:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(i.b1), i.op2.format(i.b1))
	case xmmUnaryRmR:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(false), i.op2.format(false))
	case unaryRmR:
		var suffix string
		if i.b1 {
			suffix = "q"
		} else {
			suffix = "l"
		}
		return fmt.Sprintf("%s%s %s, %s", unaryRmROpcode(i.u1), suffix, i.op1.format(i.b1), i.op2.format(i.b1))
	case not:
		var op string
		if i.b1 {
			op = "notq"
		} else {
			op = "notl"
		}
		return fmt.Sprintf("%s %s", op, i.op1.format(i.b1))
	case neg:
		var op string
		if i.b1 {
			op = "negq"
		} else {
			op = "negl"
		}
		return fmt.Sprintf("%s %s", op, i.op1.format(i.b1))
	case div:
		var prefix string
		var op string
		if i.b1 {
			op = "divq"
		} else {
			op = "divl"
		}
		if i.u1 != 0 {
			prefix = "i"
		}
		return fmt.Sprintf("%s%s %s", prefix, op, i.op1.format(i.b1))
	case mulHi:
		signed, _64 := i.u1 != 0, i.b1
		var op string
		switch {
		case signed && _64:
			op = "imulq"
		case !signed && _64:
			op = "mulq"
		case signed && !_64:
			op = "imull"
		case !signed && !_64:
			op = "mull"
		}
		return fmt.Sprintf("%s %s", op, i.op1.format(i.b1))
	case checkedDivOrRemSeq:
		panic("TODO")
	case signExtendData:
		var op string
		if i.b1 {
			op = "cqo"
		} else {
			op = "cdq"
		}
		return op
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
		var suffix string
		if i.b1 {
			suffix = "q"
		} else {
			suffix = "l"
		}
		return fmt.Sprintf("%s%s %s, %s", shiftROp(i.u1), suffix, i.op1.format(false), i.op2.format(i.b1))
	case xmmRmiReg:
		return fmt.Sprintf("%s %s, %s", sseOpcode(i.u1), i.op1.format(true), i.op2.format(true))
	case cmpRmiR:
		var op, suffix string
		if i.u1 != 0 {
			op = "cmp"
		} else {
			op = "test"
		}
		if i.b1 {
			suffix = "q"
		} else {
			suffix = "l"
		}
		if op == "test" && i.op1.kind == operandKindMem {
			// Print consistently with AT&T syntax.
			return fmt.Sprintf("%s%s %s, %s", op, suffix, i.op2.format(i.b1), i.op1.format(i.b1))
		}
		return fmt.Sprintf("%s%s %s, %s", op, suffix, i.op1.format(i.b1), i.op2.format(i.b1))
	case setcc:
		return fmt.Sprintf("set%s %s", cond(i.u1), i.op2.format(true))
	case cmove:
		var suffix string
		if i.b1 {
			suffix = "q"
		} else {
			suffix = "l"
		}
		return fmt.Sprintf("cmov%s%s %s, %s", cond(i.u1), suffix, i.op1.format(i.b1), i.op2.format(i.b1))
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
	case xmmCmpRmR:
		panic("TODO")
	case xmmRmRImm:
		panic("TODO")
	case jmp:
		return fmt.Sprintf("jmp %s", i.op1.format(true))
	case jmpIf:
		return fmt.Sprintf("j%s %s", cond(i.u1), i.op1.format(true))
	case jmpTableIsland:
		labels := make([]string, len(i.targets))
		for index, l := range i.targets {
			labels[index] = backend.Label(l).String()
		}
		return fmt.Sprintf("jump_table_island [%s]", strings.Join(labels, ", "))
	case exitSequence:
		return fmt.Sprintf("exit_sequence %s", i.op1.format(true))
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
	case v128ConstIsland:
		return fmt.Sprintf("v128ConstIsland (%#x, %#x)", i.u1, i.u2)
	case xchg:
		return fmt.Sprintf("xchg %s, %s", i.op1.format(true), i.op2.format(true))
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
	case defKindCall:
		*regs = append(*regs, i.abi.RetRealRegs...)
	case defKindRdx:
		*regs = append(*regs, rdxVReg)
	case defKindRaxRdx:
		*regs = append(*regs, raxVReg, rdxVReg)

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
	case useKindOp1Op2Reg, useKindOp1RegOp2:
		opAny, opReg := &i.op1, &i.op2
		if uk == useKindOp1RegOp2 {
			opAny, opReg = opReg, opAny
		}
		// The destination operand (op2) can be only reg,
		// the source operand (op1) can be imm32, reg or mem.
		switch opAny.kind {
		case operandKindReg:
			*regs = append(*regs, opAny.r)
		case operandKindMem:
			opAny.amode.uses(regs)
		case operandKindImm32:
		default:
			panic(fmt.Sprintf("BUG: invalid operand: %s", i))
		}
		if opReg.kind != operandKindReg {
			panic(fmt.Sprintf("BUG: invalid operand: %s", i))
		}
		*regs = append(*regs, opReg.r)
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
	case useKindCallInd:
		op := i.op1
		switch op.kind {
		case operandKindReg:
			*regs = append(*regs, op.r)
		case operandKindMem:
			op.amode.uses(regs)
		default:
			panic(fmt.Sprintf("BUG: invalid operand: %s", i))
		}
		*regs = append(*regs, i.abi.ArgRealRegs...)
	case useKindCall:
		*regs = append(*regs, i.abi.ArgRealRegs...)
	case useKindRax:
		*regs = append(*regs, raxVReg)
	case useKindOp1Rax:
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
		*regs = append(*regs, raxVReg)

	default:
		panic(fmt.Sprintf("BUG: invalid useKind %s for %s", uk, i))
	}
	return *regs
}

// AssignUse implements regalloc.Instr.
func (i *instruction) AssignUse(index int, v regalloc.VReg) {
	switch uk := useKinds[i.kind]; uk {
	case useKindNone:
	case useKindCallInd:
		if index != 0 {
			panic("BUG")
		}
		op := &i.op1
		switch op.kind {
		case operandKindReg:
			op.r = v
		case operandKindMem:
			op.amode.assignUses(index, v)
		default:
			panic("BUG")
		}
	case useKindOp1Op2Reg, useKindOp1RegOp2:
		op, opMustBeReg := &i.op1, &i.op2
		if uk == useKindOp1RegOp2 {
			op, opMustBeReg = opMustBeReg, op
		}
		switch op.kind {
		case operandKindReg:
			if index == 0 {
				if op.r.IsRealReg() {
					panic("BUG already assigned: " + i.String())
				}
				op.r = v
			} else if index == 1 {
				if opMustBeReg.r.IsRealReg() {
					panic("BUG already assigned: " + i.String())
				}
				opMustBeReg.r = v
			} else {
				panic("BUG")
			}
		case operandKindMem:
			nregs := op.amode.nregs()
			if index < nregs {
				op.amode.assignUses(index, v)
			} else if index == nregs {
				if opMustBeReg.r.IsRealReg() {
					panic("BUG already assigned: " + i.String())
				}
				opMustBeReg.r = v
			} else {
				panic("BUG")
			}
		case operandKindImm32:
			if index == 0 {
				if opMustBeReg.r.IsRealReg() {
					panic("BUG already assigned: " + i.String())
				}
				opMustBeReg.r = v
			} else {
				panic("BUG")
			}
		default:
			panic(fmt.Sprintf("BUG: invalid operand pair: %s", i))
		}
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
	case useKindOp1Rax:
		if index == 0 {
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
		} else if index == 1 {
			// Do nothing.
		} else {
			panic("BUG")
		}
	case useKindRax:
		if index != 0 {
			panic("BUG")
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
	return k == movRR || (k == xmmUnaryRmR && i.op1.kind == operandKindReg)
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

	// jmpTableIsland is to emit the jump table.
	jmpTableIsland

	// exitSequence exits the execution and go back to the Go world.
	exitSequence

	// An instruction that will always trigger the illegal instruction exception.
	ud2

	// xchg swaps the contents of two gp registers.
	// The instruction doesn't make sense before register allocation, so it doensn't
	// have useKinds and defKinds to avoid being used by the register allocator.
	xchg

	// v128ConstIsland is 16 bytes (128-bit) constant that will be loaded into an XMM.
	v128ConstIsland

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
	case xmmCmpRmR:
		return "xmmCmpRmR"
	case xmmRmRImm:
		return "xmmRmRImm"
	case jmpIf:
		return "jmpIf"
	case jmp:
		return "jmp"
	case jmpTableIsland:
		return "jmpTableIsland"
	case exitSequence:
		return "exit_sequence"
	case v128ConstIsland:
		return "v128ConstIsland"
	case ud2:
		return "ud2"
	case xchg:
		return "xchg"
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

func (i *instruction) asJmpTableSequence(targets []uint32) *instruction {
	i.kind = jmpTableIsland
	i.targets = targets
	return i
}

func (i *instruction) asJmp(target operand) *instruction {
	i.kind = jmp
	i.op1 = target
	return i
}

func (i *instruction) jmpLabel() backend.Label {
	switch i.kind {
	case jmp, jmpIf, lea:
		return i.op1.label()
	default:
		panic("BUG")
	}
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

func (i *instruction) asXmmRmR(op sseOpcode, rm operand, rd regalloc.VReg) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmRmR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
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

func (i *instruction) asSignExtendData(_64 bool) *instruction {
	i.kind = signExtendData
	i.b1 = _64
	return i
}

func (i *instruction) asUD2() *instruction {
	i.kind = ud2
	return i
}

func (i *instruction) asDiv(rn operand, signed bool, _64 bool) *instruction {
	i.kind = div
	i.op1 = rn
	i.b1 = _64
	if signed {
		i.u1 = 1
	}
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
	if rm.kind != operandKindReg && (rm.kind != operandKindMem) {
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

func (i *instruction) asShiftR(op shiftROp, amount operand, rd regalloc.VReg, _64 bool) *instruction {
	if amount.kind != operandKindReg && amount.kind != operandKindImm32 {
		panic("BUG")
	}
	i.kind = shiftR
	i.op1 = amount
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	i.b1 = _64
	return i
}

func (i *instruction) asXmmRmiReg(op sseOpcode, rm operand, rd regalloc.VReg) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindImm32 && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmRmiReg
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	return i
}

func (i *instruction) asCmpRmiR(cmp bool, rm operand, rn regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindImm32 && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = cmpRmiR
	i.op1 = rm
	i.op2 = newOperandReg(rn)
	if cmp {
		i.u1 = 1
	}
	i.b1 = _64
	return i
}

func (i *instruction) asSetcc(c cond, rd regalloc.VReg) *instruction {
	i.kind = setcc
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(c)
	return i
}

func (i *instruction) asCmove(c cond, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	i.kind = cmove
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(c)
	i.b1 = _64
	return i
}

func (i *instruction) asExitSeq(execCtx regalloc.VReg) *instruction {
	i.kind = exitSequence
	i.op1 = newOperandReg(execCtx)
	return i
}

func (i *instruction) asXmmUnaryRmR(op sseOpcode, rm operand, rd regalloc.VReg) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem {
		panic("BUG")
	}
	i.kind = xmmUnaryRmR
	i.op1 = rm
	i.op2 = newOperandReg(rd)
	i.u1 = uint64(op)
	return i
}

func (i *instruction) asV128ConstIsland(lo, hi uint64) *instruction {
	i.kind = v128ConstIsland
	i.u1 = lo
	i.u2 = hi
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

func (i *instruction) asXCHG(rm, rd regalloc.VReg) *instruction {
	i.kind = xchg
	i.op1 = newOperandReg(rm)
	i.op2 = newOperandReg(rd)
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

type shiftROp byte

const (
	shiftROpRotateLeft           shiftROp = 0
	shiftROpRotateRight          shiftROp = 1
	shiftROpShiftLeft            shiftROp = 4
	shiftROpShiftRightLogical    shiftROp = 5
	shiftROpShiftRightArithmetic shiftROp = 7
)

func (s shiftROp) String() string {
	switch s {
	case shiftROpRotateLeft:
		return "rol"
	case shiftROpRotateRight:
		return "ror"
	case shiftROpShiftLeft:
		return "shl"
	case shiftROpShiftRightLogical:
		return "shr"
	case shiftROpShiftRightArithmetic:
		return "sar"
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
	defKindCall
	defKindRdx
	defKindRaxRdx
)

var defKinds = [instrMax]defKind{
	nop0:            defKindNone,
	div:             defKindRaxRdx,
	signExtendData:  defKindRdx,
	ret:             defKindNone,
	movRR:           defKindOp2,
	movRM:           defKindNone,
	xmmMovRM:        defKindNone,
	aluRmiR:         defKindNone,
	shiftR:          defKindNone,
	imm:             defKindOp2,
	unaryRmR:        defKindOp2,
	xmmUnaryRmR:     defKindOp2,
	xmmRmR:          defKindNone,
	mov64MR:         defKindOp2,
	movsxRmR:        defKindOp2,
	movzxRmR:        defKindOp2,
	gprToXmm:        defKindOp2,
	cmove:           defKindNone,
	call:            defKindCall,
	callIndirect:    defKindCall,
	ud2:             defKindNone,
	jmp:             defKindNone,
	jmpIf:           defKindNone,
	jmpTableIsland:  defKindNone,
	cmpRmiR:         defKindNone,
	exitSequence:    defKindNone,
	lea:             defKindOp2,
	v128ConstIsland: defKindNone,
	setcc:           defKindOp2,
}

// String implements fmt.Stringer.
func (d defKind) String() string {
	switch d {
	case defKindNone:
		return "none"
	case defKindOp2:
		return "op2"
	case defKindCall:
		return "call"
	case defKindRdx:
		return "rdx"
	case defKindRaxRdx:
		return "raxrdx"
	default:
		return "invalid"
	}
}

type useKind byte

const (
	useKindNone useKind = iota + 1
	useKindOp1
	// useKindOp1Op2Reg is Op1 can be any operand, Op2 must be a register.
	useKindOp1Op2Reg
	// useKindOp1RegOp2 is Op1 must be a register, Op2 can be any operand.
	useKindOp1RegOp2
	// useKindRax is %rax is used (for instance in signExtendData).
	useKindRax
	// useKindOp1Rax is Op1 must be a reg, mem operand; the other operand is implicitly %rax.
	useKindOp1Rax
	useKindCall
	useKindCallInd
)

var useKinds = [instrMax]useKind{
	nop0:            useKindNone,
	div:             useKindOp1Rax,
	signExtendData:  useKindRax,
	ret:             useKindNone,
	movRR:           useKindOp1,
	movRM:           useKindOp1RegOp2,
	xmmMovRM:        useKindOp1RegOp2,
	cmove:           useKindOp1Op2Reg,
	aluRmiR:         useKindOp1Op2Reg,
	shiftR:          useKindOp1Op2Reg,
	imm:             useKindNone,
	unaryRmR:        useKindOp1,
	xmmUnaryRmR:     useKindOp1,
	xmmRmR:          useKindOp1Op2Reg,
	mov64MR:         useKindOp1,
	movzxRmR:        useKindOp1,
	movsxRmR:        useKindOp1,
	gprToXmm:        useKindOp1,
	call:            useKindCall,
	callIndirect:    useKindCallInd,
	ud2:             useKindNone,
	jmpIf:           useKindOp1,
	jmp:             useKindOp1,
	cmpRmiR:         useKindOp1Op2Reg,
	exitSequence:    useKindOp1,
	lea:             useKindOp1,
	v128ConstIsland: useKindNone,
	jmpTableIsland:  useKindNone,
	setcc:           useKindNone,
}

func (u useKind) String() string {
	switch u {
	case useKindNone:
		return "none"
	case useKindOp1:
		return "op1"
	case useKindOp1Op2Reg:
		return "op1op2Reg"
	case useKindOp1RegOp2:
		return "op1RegOp2"
	case useKindCall:
		return "call"
	case useKindCallInd:
		return "callInd"
	case useKindOp1Rax:
		return "op1rax"
	case useKindRax:
		return "rax"
	default:
		return "invalid"
	}
}
