package asm_amd64

import "github.com/tetratelabs/wazero/internal/asm"

// AMD64-specific conditional register states.
// https://www.lri.fr/~filliatr/ens/compil/x86-64.pdf
// https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-instruction-set-reference-manual-325383.pdf
const (
	ConditionalRegisterStateE  = asm.ConditionalRegisterStateUnset + 1 + iota // ZF equal to zero
	ConditionalRegisterStateNE                                                //˜ZF not equal to zero
	ConditionalRegisterStateS                                                 // SF negative
	ConditionalRegisterStateNS                                                // ˜SF non-negative
	ConditionalRegisterStateG                                                 // ˜(SF xor OF) & ˜ ZF greater (signed >)
	ConditionalRegisterStateGE                                                // ˜(SF xor OF) greater or equal (signed >=)
	ConditionalRegisterStateL                                                 // SF xor OF less (signed <)
	ConditionalRegisterStateLE                                                // (SF xor OF) | ZF less or equal (signed <=)
	ConditionalRegisterStateA                                                 // ˜CF & ˜ZF above (unsigned >)
	ConditionalRegisterStateAE                                                // ˜CF above or equal (unsigned >=)
	ConditionalRegisterStateB                                                 // CF below (unsigned <)
	ConditionalRegisterStateBE                                                // CF | ZF below or equal (unsigned <=)
)

// AMD64-specific instructions.
// https://www.felixcloutier.com/x86/index.html
//
// Note: here we do not define all of amd64 instructions, and we only define the ones used by wazero's JIT compiler.
// Note: naming convension is exactly the same as Go assembler: https://go.dev/doc/asm
const (
	NONE asm.Instruction = iota
	ADDL
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
	RET
	JMP
	NOP
	UD2
)

func InstructionName(instruction asm.Instruction) string {
	switch instruction {
	case ADDL:
		return "ADDL"
	case ADDQ:
		return "ADDQ"
	case ADDSD:
		return "ADDSD"
	case ADDSS:
		return "ADDSS"
	case ANDL:
		return "ANDL"
	case ANDPD:
		return "ANDPD"
	case ANDPS:
		return "ANDPS"
	case ANDQ:
		return "ANDQ"
	case BSRL:
		return "BSRL"
	case BSRQ:
		return "BSRQ"
	case CDQ:
		return "CDQ"
	case CMOVQCS:
		return "CMOVQCS"
	case CMPL:
		return "CMPL"
	case CMPQ:
		return "CMPQ"
	case COMISD:
		return "COMISD"
	case COMISS:
		return "COMISS"
	case CQO:
		return "CQO"
	case CVTSD2SS:
		return "CVTSD2SS"
	case CVTSL2SD:
		return "CVTSL2SD"
	case CVTSL2SS:
		return "CVTSL2SS"
	case CVTSQ2SD:
		return "CVTSQ2SD"
	case CVTSQ2SS:
		return "CVTSQ2SS"
	case CVTSS2SD:
		return "CVTSS2SD"
	case CVTTSD2SL:
		return "CVTTSD2SL"
	case CVTTSD2SQ:
		return "CVTTSD2SQ"
	case CVTTSS2SL:
		return "CVTTSS2SL"
	case CVTTSS2SQ:
		return "CVTTSS2SQ"
	case DECQ:
		return "DECQ"
	case DIVL:
		return "DIVL"
	case DIVQ:
		return "DIVQ"
	case DIVSD:
		return "DIVSD"
	case DIVSS:
		return "DIVSS"
	case IDIVL:
		return "IDIVL"
	case IDIVQ:
		return "IDIVQ"
	case INCQ:
		return "INCQ"
	case JCC:
		return "JCC"
	case JCS:
		return "JCS"
	case JEQ:
		return "JEQ"
	case JGE:
		return "JGE"
	case JGT:
		return "JGT"
	case JHI:
		return "JHI"
	case JLE:
		return "JLE"
	case JLS:
		return "JLS"
	case JLT:
		return "JLT"
	case JMI:
		return "JMI"
	case JNE:
		return "JNE"
	case JPC:
		return "JPC"
	case JPL:
		return "JPL"
	case JPS:
		return "JPS"
	case LEAQ:
		return "LEAQ"
	case LZCNTL:
		return "LZCNTL"
	case LZCNTQ:
		return "LZCNTQ"
	case MAXSD:
		return "MAXSD"
	case MAXSS:
		return "MAXSS"
	case MINSD:
		return "MINSD"
	case MINSS:
		return "MINSS"
	case MOVB:
		return "MOVB"
	case MOVBLSX:
		return "MOVBLSX"
	case MOVBLZX:
		return "MOVBLZX"
	case MOVBQSX:
		return "MOVBQSX"
	case MOVBQZX:
		return "MOVBQZX"
	case MOVL:
		return "MOVL"
	case MOVLQSX:
		return "MOVLQSX"
	case MOVLQZX:
		return "MOVLQZX"
	case MOVQ:
		return "MOVQ"
	case MOVW:
		return "MOVW"
	case MOVWLSX:
		return "MOVWLSX"
	case MOVWLZX:
		return "MOVWLZX"
	case MOVWQSX:
		return "MOVWQSX"
	case MOVWQZX:
		return "MOVWQZX"
	case MULL:
		return "MULL"
	case MULQ:
		return "MULQ"
	case MULSD:
		return "MULSD"
	case MULSS:
		return "MULSS"
	case ORL:
		return "ORL"
	case ORPD:
		return "ORPD"
	case ORPS:
		return "ORPS"
	case ORQ:
		return "ORQ"
	case POPCNTL:
		return "POPCNTL"
	case POPCNTQ:
		return "POPCNTQ"
	case PSLLL:
		return "PSLLL"
	case PSLLQ:
		return "PSLLQ"
	case PSRLL:
		return "PSRLL"
	case PSRLQ:
		return "PSRLQ"
	case ROLL:
		return "ROLL"
	case ROLQ:
		return "ROLQ"
	case RORL:
		return "RORL"
	case RORQ:
		return "RORQ"
	case ROUNDSD:
		return "ROUNDSD"
	case ROUNDSS:
		return "ROUNDSS"
	case SARL:
		return "SARL"
	case SARQ:
		return "SARQ"
	case SETCC:
		return "SETCC"
	case SETCS:
		return "SETCS"
	case SETEQ:
		return "SETEQ"
	case SETGE:
		return "SETGE"
	case SETGT:
		return "SETGT"
	case SETHI:
		return "SETHI"
	case SETLE:
		return "SETLE"
	case SETLS:
		return "SETLS"
	case SETLT:
		return "SETLT"
	case SETMI:
		return "SETMI"
	case SETNE:
		return "SETNE"
	case SETPC:
		return "SETPC"
	case SETPL:
		return "SETPL"
	case SETPS:
		return "SETPS"
	case SHLL:
		return "SHLL"
	case SHLQ:
		return "SHLQ"
	case SHRL:
		return "SHRL"
	case SHRQ:
		return "SHRQ"
	case SQRTSD:
		return "SQRTSD"
	case SQRTSS:
		return "SQRTSS"
	case SUBL:
		return "SUBL"
	case SUBQ:
		return "SUBQ"
	case SUBSD:
		return "SUBSD"
	case SUBSS:
		return "SUBSS"
	case TESTL:
		return "TESTL"
	case TESTQ:
		return "TESTQ"
	case TZCNTL:
		return "TZCNTL"
	case TZCNTQ:
		return "TZCNTQ"
	case UCOMISD:
		return "UCOMISD"
	case UCOMISS:
		return "UCOMISS"
	case XORL:
		return "XORL"
	case XORPD:
		return "XORPD"
	case XORPS:
		return "XORPS"
	case XORQ:
		return "XORQ"
	case RET:
		return "RET"
	case JMP:
		return "JMP"
	case NOP:
		return "NOP"
	case UD2:
		return "UD2"
	}
	return "Unknown"
}

// Arm64-specific registers.
// https://www.lri.fr/~filliatr/ens/compil/x86-64.pdf
// https://cs.brown.edu/courses/cs033/docs/guides/x64_cheatsheet.pdf
//
// Note: naming convension is exactly the same as Go assembler: https://go.dev/doc/asm
const (
	REG_AX asm.Register = asm.NilRegister + 1 + iota
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
	REG_X0
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
)

func RegisterName(reg asm.Register) string {
	switch reg {
	case REG_AX:
		return "AX"
	case REG_CX:
		return "CX"
	case REG_DX:
		return "DX"
	case REG_BX:
		return "BX"
	case REG_SP:
		return "SP"
	case REG_BP:
		return "BP"
	case REG_SI:
		return "SI"
	case REG_DI:
		return "DI"
	case REG_R8:
		return "R8"
	case REG_R9:
		return "R9"
	case REG_R10:
		return "R10"
	case REG_R11:
		return "R11"
	case REG_R12:
		return "R12"
	case REG_R13:
		return "R13"
	case REG_R14:
		return "R14"
	case REG_R15:
		return "R15"
	case REG_X0:
		return "X0"
	case REG_X1:
		return "X1"
	case REG_X2:
		return "X2"
	case REG_X3:
		return "X3"
	case REG_X4:
		return "X4"
	case REG_X5:
		return "X5"
	case REG_X6:
		return "X6"
	case REG_X7:
		return "X7"
	case REG_X8:
		return "X8"
	case REG_X9:
		return "X9"
	case REG_X10:
		return "X10"
	case REG_X11:
		return "X11"
	case REG_X12:
		return "X12"
	case REG_X13:
		return "X13"
	case REG_X14:
		return "X14"
	case REG_X15:
		return "X15"
	default:
		return "nil"
	}
}
