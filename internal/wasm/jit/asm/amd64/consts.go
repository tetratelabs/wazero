package amd64

import "github.com/tetratelabs/wazero/internal/wasm/jit/asm"

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

const (
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
	RET
	JMP
	NOP
	UD2
)

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
