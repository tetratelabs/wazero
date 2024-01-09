package amd64

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
)

type instruction struct {
	kind                instructionKind
	prev, next          *instruction
	abi                 *backend.FunctionABI
	op1, op2            operand
	u1                  uint64
	b1                  bool
	addedBeforeRegAlloc bool
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
			return fmt.Sprintf("movabsq $%d, %s", int64(i.u1), i.op1.format(true))
		} else {
			return fmt.Sprintf("movl $%d, %s", int32(i.u1), i.op1.format(false))
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
	case loadEffectiveAddress:
		panic("TODO")
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
		panic("TODO")
	case pop64:
		panic("TODO")
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
		panic("TODO")
	default:
		panic(fmt.Sprintf("BUG: %v", i.kind))
	}
}

// Defs implements regalloc.Instr.
func (i *instruction) Defs(i2 *[]regalloc.VReg) []regalloc.VReg {
	// TODO implement me
	panic("implement me")
}

// Uses implements regalloc.Instr.
func (i *instruction) Uses(i2 *[]regalloc.VReg) []regalloc.VReg {
	// TODO implement me
	panic("implement me")
}

// AssignUse implements regalloc.Instr.
func (i *instruction) AssignUse(index int, v regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// AssignDef implements regalloc.Instr.
func (i *instruction) AssignDef(reg regalloc.VReg) {
	// TODO implement me
	panic("implement me")
}

// IsCopy implements regalloc.Instr.
func (i *instruction) IsCopy() bool {
	// TODO implement me
	panic("implement me")
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

	// Zero-extended loads, except for 64 bits: movz (bl bq wl wq lq) addr reg.
	// Note that the lq variant doesn't really exist since the default zero-extend rule makes it
	// unnecessary. For that case we emit the equivalent "movl AM, reg32".
	movzxRmR

	// A plain 64-bit integer load, since MovZX_RM_R can't represent that.
	mov64MR

	// Loads the memory address of addr into dst.
	loadEffectiveAddress

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

	// XMM (scalar or vector) unary op (from xmm to reg/mem): stores, movd, movq
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
	call

	// Indirect call: callq (reg mem).
	callIndirect

	// Return.
	ret

	// Jump to a known target: jmp simm32.
	jmpKnown

	// One-way conditional branch: jcond cond target.
	///
	// This instruction is useful when we have conditional jumps depending on more than two
	// conditions, see for instance the lowering of Brz/brnz with Fcmp inputs.
	///
	// A note of caution: in contexts where the branch target is another block, this has to be the
	// same successor as the one specified in the terminator branch of the current block.
	// Otherwise, this might confuse register allocation by creating new invisible edges.
	jmpIf

	// Two-way conditional branch: jcond cond target target.
	// Emitted as a compound sequence; the MachBuffer will shrink it as appropriate.
	jmpCond

	// Jump-table sequence, as one compound instruction (see note in lower.rs for rationale).
	// The generated code sequence is described in the emit's function match arm for this
	// instruction.
	// See comment in lowering about the temporaries signedness.
	jmpTableSeq

	// Indirect jump: jmpq (reg mem).
	jmpUnknown

	// Traps if the condition code is set.
	trapIf

	// An instruction that will always trigger the illegal instruction exception.
	ud2
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
	case loadEffectiveAddress:
		return "loadEffectiveAddress"
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
	case jmpKnown:
		return "jmpKnown"
	case jmpIf:
		return "jmpIf"
	case jmpCond:
		return "jmpCond"
	case jmpTableSeq:
		return "jmpTableSeq"
	case jmpUnknown:
		return "jmpUnknown"
	case trapIf:
		return "trapIf"
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
		return "mul"
	default:
		panic("BUG")
	}
}

func (i *instruction) asRet(abi *backend.FunctionABI) *instruction {
	i.kind = ret
	i.abi = abi
	return i
}

func (i *instruction) asImm(dst regalloc.VReg, value uint64, _64 bool) *instruction {
	i.kind = imm
	i.op1 = newOperandReg(dst)
	i.u1 = value
	i.b1 = _64
	return i
}

func (i *instruction) asAluRmiR(op aluRmiROpcode, rm operand, rd regalloc.VReg, _64 bool) *instruction {
	if rm.kind != operandKindReg && rm.kind != operandKindMem && rm.kind != operandImm32 {
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

func (i *instruction) asMovRR(rm, rd regalloc.VReg, _64 bool) *instruction {
	i.kind = movRR
	i.op1 = newOperandReg(rm)
	i.op2 = newOperandReg(rd)
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
