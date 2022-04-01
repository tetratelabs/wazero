package asm_arm64

import (
	"github.com/tetratelabs/wazero/internal/asm"
)

// Arm64-specific register states.
// https://community.arm.com/arm-community-blogs/b/architectures-and-processors-blog/posts/condition-codes-1-condition-flags-and-codes
// Note: naming convension is exactly the same as Go assembler: https://go.dev/doc/asm
const (
	COND_EQ asm.ConditionalRegisterState = asm.ConditionalRegisterStateUnset + 1 + iota
	COND_NE
	COND_HS
	COND_LO
	COND_MI
	COND_PL
	COND_VS
	COND_VC
	COND_HI
	COND_LS
	COND_GE
	COND_LT
	COND_GT
	COND_LE
	COND_AL
	COND_NV
)

// Arm64-specific registers.
// https://developer.arm.com/documentation/dui0801/a/Overview-of-AArch64-state/Predeclared-core-register-names-in-AArch64-state
// Note: naming convension is exactly the same as Go assembler: https://go.dev/doc/asm
const (
	// Integer registers.

	REG_R0 asm.Register = asm.NilRegister + 1 + iota
	REG_R1
	REG_R2
	REG_R3
	REG_R4
	REG_R5
	REG_R6
	REG_R7
	REG_R8
	REG_R9
	REG_R10
	REG_R11
	REG_R12
	REG_R13
	REG_R14
	REG_R15
	REG_R16
	REG_R17
	REG_R18
	REG_R19
	REG_R20
	REG_R21
	REG_R22
	REG_R23
	REG_R24
	REG_R25
	REG_R26
	REG_R27
	REG_R28
	REG_R29
	REG_R30
	REGZERO

	// Scalar floating point registers.

	REG_F0
	REG_F1
	REG_F2
	REG_F3
	REG_F4
	REG_F5
	REG_F6
	REG_F7
	REG_F8
	REG_F9
	REG_F10
	REG_F11
	REG_F12
	REG_F13
	REG_F14
	REG_F15
	REG_F16
	REG_F17
	REG_F18
	REG_F19
	REG_F20
	REG_F21
	REG_F22
	REG_F23
	REG_F24
	REG_F25
	REG_F26
	REG_F27
	REG_F28
	REG_F29
	REG_F30
	REG_F31

	// Floating point status register.

	REG_FPSR
)

// Arm64-specific instructions.
//
// Note: naming convension is exactly the same as Go assembler: https://go.dev/doc/asm
const (
	NOP asm.Instruction = iota
	RET
	ADD
	ADDW
	ADR
	AND
	ANDW
	ASR
	ASRW
	B
	BEQ
	BGE
	BGT
	BHI
	BHS
	BLE
	BLO
	BLS
	BLT
	BMI
	BNE
	BVS
	CLZ
	CLZW
	CMP
	CMPW
	CSET
	EOR
	EORW
	FABSD
	FABSS
	FADDD
	FADDS
	FCMPD
	FCMPS
	FCVTDS
	FCVTSD
	FCVTZSD
	FCVTZSDW
	FCVTZSS
	FCVTZSSW
	FCVTZUD
	FCVTZUDW
	FCVTZUS
	FCVTZUSW
	FDIVD
	FDIVS
	FMAXD
	FMAXS
	FMIND
	FMINS
	FMOVD
	FMOVS
	FMULD
	FMULS
	FNEGD
	FNEGS
	FRINTMD
	FRINTMS
	FRINTND
	FRINTNS
	FRINTPD
	FRINTPS
	FRINTZD
	FRINTZS
	FSQRTD
	FSQRTS
	FSUBD
	FSUBS
	LSL
	LSLW
	LSR
	LSRW
	MOVB
	MOVBU
	MOVD
	MOVH
	MOVHU
	MOVW
	MOVWU
	MRS
	MSR
	MSUB
	MSUBW
	MUL
	MULW
	NEG
	NEGW
	ORR
	ORRW
	RBIT
	RBITW
	RNG
	ROR
	RORW
	SCVTFD
	SCVTFS
	SCVTFWD
	SCVTFWS
	SDIV
	SDIVW
	SUB
	SUBS
	SUBW
	SXTB
	SXTBW
	SXTH
	SXTHW
	SXTW
	UCVTFD
	UCVTFS
	UCVTFWD
	UCVTFWS
	UDIV
	UDIVW
	UXTW
	VBIT
	VCNT
	VUADDLV
)
