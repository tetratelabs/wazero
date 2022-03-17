package arm64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

// SimdRegisterForScalarFloatRegister returns SIMD register which corresponds to the given scalar float register.
// In other words, this returns: REG_F0 -> REG_V0, REG_F1 -> REG_V1, ...., REG_F31 -> REG_V31.
func SimdRegisterForScalarFloatRegister(freg asm.Register) asm.Register {
	return freg + (REG_F31 - REG_F0) + 1
}

const (
	// integer
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

	// scalar floating point
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

	// SIMD
	REG_V0
	REG_V1
	REG_V2
	REG_V3
	REG_V4
	REG_V5
	REG_V6
	REG_V7
	REG_V8
	REG_V9
	REG_V10
	REG_V11
	REG_V12
	REG_V13
	REG_V14
	REG_V15
	REG_V16
	REG_V17
	REG_V18
	REG_V19
	REG_V20
	REG_V21
	REG_V22
	REG_V23
	REG_V24
	REG_V25
	REG_V26
	REG_V27
	REG_V28
	REG_V29
	REG_V30
	REG_V31
)

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
