package amd64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

const (

	// Instrctuins in x86 pkg.
	ADDL = iota
	ADDQ
	ADDSD
	ADDSS
	ANDL
	ANDPD
	ANDPS
	ANDQ
	BSRL
	BSRQ
	CDQ
	CMOVQCS
	CMPL
	CMPQ
	COMISD
	COMISS
	CQO
	CVTSD2SS
	CVTSL2SD
	CVTSL2SS
	CVTSQ2SD
	CVTSQ2SS
	CVTSS2SD
	CVTTSD2SL
	CVTTSD2SQ
	CVTTSS2SL
	CVTTSS2SQ
	DECQ
	DIVL
	DIVQ
	DIVSD
	DIVSS
	IDIVL
	IDIVQ
	INCQ
	JCC
	JCS
	JEQ
	JGE
	JGT
	JHI
	JLE
	JLS
	JLT
	JMI
	JNE
	JPC
	JPL
	JPS
	LEAQ
	LZCNTL
	LZCNTQ
	MAXSD
	MAXSS
	MINSD
	MINSS
	MOVB
	MOVBLSX
	MOVBLZX
	MOVBQSX
	MOVBQZX
	MOVL
	MOVLQSX
	MOVLQZX
	MOVQ
	MOVW
	MOVWLSX
	MOVWLZX
	MOVWQSX
	MOVWQZX
	MULL
	MULQ
	MULSD
	MULSS
	ORL
	ORPD
	ORPS
	ORQ
	POPCNTL
	POPCNTQ
	PSLLL
	PSLLQ
	PSRLL
	PSRLQ
	ROLL
	ROLQ
	RORL
	RORQ
	ROUNDSD
	ROUNDSS
	SARL
	SARQ
	SET
	SETCC
	SETCS
	SETEQ
	SETGE
	SETGT
	SETHI
	SETLE
	SETLS
	SETLT
	SETMI
	SETNE
	SETPC
	SETPL
	SETPS
	SHLL
	SHLQ
	SHRL
	SHRQ
	SQRTSD
	SQRTSS
	SUBL
	SUBQ
	SUBSD
	SUBSS
	TESTL
	TESTQ
	TZCNTL
	TZCNTQ
	UCOMISD
	UCOMISS
	XORL
	XORPD
	XORPS
	XORQ

	// Instructions in obj pkg.
	RET
	JMP
	NOP
)

const (
	intRegisterIotaBegin   asm.Register = 2064
	floatRegisterIotaBegin asm.Register = 2108
)

const (
	REG_AX asm.Register = intRegisterIotaBegin + iota
	REG_CX
	REG_DX
	REG_BX
	REG_SP
	REG_BP
	REG_SI
	REG_DI
	REG_R8
	REG_R9
	REG_R10
	REG_R11
	REG_R12
	REG_R13
	REG_R14
	REG_R15
)

const (
	REG_X0 asm.Register = floatRegisterIotaBegin + iota
	REG_X1
	REG_X2
	REG_X3
	REG_X4
	REG_X5
	REG_X6
	REG_X7
	REG_X8
	REG_X9
	REG_X10
	REG_X11
	REG_X12
	REG_X13
	REG_X14
	REG_X15
	REG_X16
	REG_X17
	REG_X18
	REG_X19
	REG_X20
	REG_X21
	REG_X22
	REG_X23
	REG_X24
	REG_X25
	REG_X26
	REG_X27
	REG_X28
	REG_X29
	REG_X30
	REG_X31
)

func NewAssembler() (Assembler, error) {
	return newGolangAsmAssembler() // TODO: replace our homemade assembler #233
}

type Assembler interface {
	asm.AssemblerBase

	// TODO
	CompileStandAloneInstruction(asm.Instruction) asm.Node
	// TODO
	CompileRegisterToRegisterInstruction(inst asm.Instruction, from, to asm.Register)
	// TODO
	CompileMemoryToRegisterInstruction(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	// TODO
	CompileMemoryWithIndexToRegisterInstruction(inst asm.Register, sourceBaseReg asm.Register, sourceOffsetConst int64, sourceIndex asm.Register, sourceScale asm.Register, destinationReg asm.Register)
	// TODO
	CompileRegisterToMemoryWithIndexInstruction(inst asm.Register, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale asm.Register)
	// TODO
	CompileRegisterToMemoryInstruction(inst asm.Register, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64)
	// TODO
	CompileConstToRegisterInstruction(inst asm.Register, constValue int64, destinationRegister asm.Register) asm.Node
	// TODO
	CompileRegisterToConstInstruction(inst asm.Register, srcRegister asm.Register, constValue int64) asm.Node
	// TODO
	CompileRegisterToNoneInstruction(inst asm.Register, register asm.Register)
	// TODO
	CompileNoneToRegisterInstruction(inst asm.Register, register asm.Register)
	// TODO
	CompileNoneToMemoryInstruction(inst asm.Register, baseReg asm.Register, offset int64)
	// TODO
	CompileConstToMemoryInstruction(inst asm.Register, constValue int64, baseReg asm.Register, offset int64) asm.Node
	// TODO
	CompileMemoryToConstInstruction(inst asm.Register, baseReg asm.Register, offset int64, constValue int64) asm.Node
	// TODO
	CompileUnconditionalJump() asm.Node
	// TODO
	CompileJump(jmpInst asm.Instruction) asm.Node
	// TODO
	CompileJumpToRegister(reg asm.Register)
	// TODO
	CompileJumpToMemory(baseReg asm.Register, offset int64)
	// TODO
	CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction)
}
