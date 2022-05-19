package arm64

import (
	"github.com/tetratelabs/wazero/internal/asm"
)

// Arm64-specific register states.
//
// Note: Naming conventions intentionally match the Go assembler: https://go.dev/doc/asm
// See https://community.arm.com/arm-community-blogs/b/architectures-and-processors-blog/posts/condition-codes-1-condition-flags-and-codes
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
//
// Note: Naming conventions intentionally match the Go assembler: https://go.dev/doc/asm
// See https://developer.arm.com/documentation/dui0801/a/Overview-of-AArch64-state/Predeclared-core-register-names-in-AArch64-state
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

	// Floating point status register.

	REG_FPSR

	// Assign each conditional register state to the unique register ID.
	// This is to reduce the size of NodeImpl struct without having dedicated field
	// for conditional register state which would not be used by most nodes.

	REG_COND_EQ
	REG_COND_NE
	REG_COND_HS
	REG_COND_LO
	REG_COND_MI
	REG_COND_PL
	REG_COND_VS
	REG_COND_VC
	REG_COND_HI
	REG_COND_LS
	REG_COND_GE
	REG_COND_LT
	REG_COND_GT
	REG_COND_LE
	REG_COND_AL
	REG_COND_NV
)

// conditionalRegisterStateToRegister cast a conditional register to its unique register ID.
// See the comment on REG_COND_EQ above.
func conditionalRegisterStateToRegister(c asm.ConditionalRegisterState) asm.Register {
	switch c {
	case COND_EQ:
		return REG_COND_EQ
	case COND_NE:
		return REG_COND_NE
	case COND_HS:
		return REG_COND_HS
	case COND_LO:
		return REG_COND_LO
	case COND_MI:
		return REG_COND_MI
	case COND_PL:
		return REG_COND_PL
	case COND_VS:
		return REG_COND_VS
	case COND_VC:
		return REG_COND_VC
	case COND_HI:
		return REG_COND_HI
	case COND_LS:
		return REG_COND_LS
	case COND_GE:
		return REG_COND_GE
	case COND_LT:
		return REG_COND_LT
	case COND_GT:
		return REG_COND_GT
	case COND_LE:
		return REG_COND_LE
	case COND_AL:
		return REG_COND_AL
	case COND_NV:
		return REG_COND_NV
	}
	return asm.NilRegister
}

func RegisterName(r asm.Register) string {
	switch r {
	case asm.NilRegister:
		return "nil"
	case REG_R0:
		return "R0"
	case REG_R1:
		return "R1"
	case REG_R2:
		return "R2"
	case REG_R3:
		return "R3"
	case REG_R4:
		return "R4"
	case REG_R5:
		return "R5"
	case REG_R6:
		return "R6"
	case REG_R7:
		return "R7"
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
	case REG_R16:
		return "R16"
	case REG_R17:
		return "R17"
	case REG_R18:
		return "R18"
	case REG_R19:
		return "R19"
	case REG_R20:
		return "R20"
	case REG_R21:
		return "R21"
	case REG_R22:
		return "R22"
	case REG_R23:
		return "R23"
	case REG_R24:
		return "R24"
	case REG_R25:
		return "R25"
	case REG_R26:
		return "R26"
	case REG_R27:
		return "R27"
	case REG_R28:
		return "R28"
	case REG_R29:
		return "R29"
	case REG_R30:
		return "R30"
	case REGZERO:
		return "ZERO"
	case REG_V0:
		return "V0"
	case REG_V1:
		return "V1"
	case REG_V2:
		return "V2"
	case REG_V3:
		return "V3"
	case REG_V4:
		return "V4"
	case REG_V5:
		return "V5"
	case REG_V6:
		return "V6"
	case REG_V7:
		return "V7"
	case REG_V8:
		return "V8"
	case REG_V9:
		return "V9"
	case REG_V10:
		return "V10"
	case REG_V11:
		return "V11"
	case REG_V12:
		return "V12"
	case REG_V13:
		return "V13"
	case REG_V14:
		return "V14"
	case REG_V15:
		return "V15"
	case REG_V16:
		return "V16"
	case REG_V17:
		return "V17"
	case REG_V18:
		return "V18"
	case REG_V19:
		return "V19"
	case REG_V20:
		return "V20"
	case REG_V21:
		return "V21"
	case REG_V22:
		return "V22"
	case REG_V23:
		return "V23"
	case REG_V24:
		return "V24"
	case REG_V25:
		return "V25"
	case REG_V26:
		return "V26"
	case REG_V27:
		return "V27"
	case REG_V28:
		return "V28"
	case REG_V29:
		return "V29"
	case REG_V30:
		return "V30"
	case REG_V31:
		return "V31"
	case REG_FPSR:
		return "FPSR"
	case REG_COND_EQ:
		return "COND_EQ"
	case REG_COND_NE:
		return "COND_NE"
	case REG_COND_HS:
		return "COND_HS"
	case REG_COND_LO:
		return "COND_LO"
	case REG_COND_MI:
		return "COND_MI"
	case REG_COND_PL:
		return "COND_PL"
	case REG_COND_VS:
		return "COND_VS"
	case REG_COND_VC:
		return "COND_VC"
	case REG_COND_HI:
		return "COND_HI"
	case REG_COND_LS:
		return "COND_LS"
	case REG_COND_GE:
		return "COND_GE"
	case REG_COND_LT:
		return "COND_LT"
	case REG_COND_GT:
		return "COND_GT"
	case REG_COND_LE:
		return "COND_LE"
	case REG_COND_AL:
		return "COND_AL"
	case REG_COND_NV:
		return "COND_NV"
	}
	return "UNKNOWN"
}

// Arm64-specific instructions.
//
// Note: This only defines arm64 instructions used by wazero's compiler.
// Note: Naming conventions intentionally match the Go assembler: https://go.dev/doc/asm
const (
	NOP asm.Instruction = iota
	RET
	ADD
	ADDS
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
	BPL
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

	// Vector instructions.
	VBIT
	VCNT
	VMOV
	VUADDLV
	VLD1
	VST1
	VADD
	VFADDS
	VFADDD
)

type VectorArrangement byte

const (
	VectorArrangementNone VectorArrangement = iota
	VectorArrangement8B
	VectorArrangement16B
	VectorArrangement4H
	VectorArrangement8H
	VectorArrangement2S
	VectorArrangement4S
	VectorArrangement1D
	VectorArrangement2D
	VectorArrangementB
	VectorArrangementH
	VectorArrangementS
	VectorArrangementD
)

func (v VectorArrangement) String() (ret string) {
	switch v {
	case VectorArrangement8B:
		ret = "8B"
	case VectorArrangement16B:
		ret = "16B"
	case VectorArrangement4H:
		ret = "4H"
	case VectorArrangement8H:
		ret = "8H"
	case VectorArrangement2S:
		ret = "2S"
	case VectorArrangement4S:
		ret = "4S"
	case VectorArrangement1D:
		ret = "1D"
	case VectorArrangement2D:
		ret = "2D"
	case VectorArrangementB:
		ret = ""
	case VectorArrangementH:
		ret = "H"
	case VectorArrangementS:
		ret = "S"
	case VectorArrangementD:
		ret = "D"
	default:
		ret = "unknown"
	}
	return
}

type VectorIndex byte

func InstructionName(i asm.Instruction) string {
	switch i {
	case NOP:
		return "NOP"
	case RET:
		return "RET"
	case ADD:
		return "ADD"
	case ADDS:
		return "ADDS"
	case ADDW:
		return "ADDW"
	case ADR:
		return "ADR"
	case AND:
		return "AND"
	case ANDW:
		return "ANDW"
	case ASR:
		return "ASR"
	case ASRW:
		return "ASRW"
	case B:
		return "B"
	case BEQ:
		return "BEQ"
	case BGE:
		return "BGE"
	case BGT:
		return "BGT"
	case BHI:
		return "BHI"
	case BHS:
		return "BHS"
	case BLE:
		return "BLE"
	case BLO:
		return "BLO"
	case BLS:
		return "BLS"
	case BLT:
		return "BLT"
	case BMI:
		return "BMI"
	case BPL:
		return "BPL"
	case BNE:
		return "BNE"
	case BVS:
		return "BVS"
	case CLZ:
		return "CLZ"
	case CLZW:
		return "CLZW"
	case CMP:
		return "CMP"
	case CMPW:
		return "CMPW"
	case CSET:
		return "CSET"
	case EOR:
		return "EOR"
	case EORW:
		return "EORW"
	case FABSD:
		return "FABSD"
	case FABSS:
		return "FABSS"
	case FADDD:
		return "FADDD"
	case FADDS:
		return "FADDS"
	case FCMPD:
		return "FCMPD"
	case FCMPS:
		return "FCMPS"
	case FCVTDS:
		return "FCVTDS"
	case FCVTSD:
		return "FCVTSD"
	case FCVTZSD:
		return "FCVTZSD"
	case FCVTZSDW:
		return "FCVTZSDW"
	case FCVTZSS:
		return "FCVTZSS"
	case FCVTZSSW:
		return "FCVTZSSW"
	case FCVTZUD:
		return "FCVTZUD"
	case FCVTZUDW:
		return "FCVTZUDW"
	case FCVTZUS:
		return "FCVTZUS"
	case FCVTZUSW:
		return "FCVTZUSW"
	case FDIVD:
		return "FDIVD"
	case FDIVS:
		return "FDIVS"
	case FMAXD:
		return "FMAXD"
	case FMAXS:
		return "FMAXS"
	case FMIND:
		return "FMIND"
	case FMINS:
		return "FMINS"
	case FMOVD:
		return "FMOVD"
	case FMOVS:
		return "FMOVS"
	case FMULD:
		return "FMULD"
	case FMULS:
		return "FMULS"
	case FNEGD:
		return "FNEGD"
	case FNEGS:
		return "FNEGS"
	case FRINTMD:
		return "FRINTMD"
	case FRINTMS:
		return "FRINTMS"
	case FRINTND:
		return "FRINTND"
	case FRINTNS:
		return "FRINTNS"
	case FRINTPD:
		return "FRINTPD"
	case FRINTPS:
		return "FRINTPS"
	case FRINTZD:
		return "FRINTZD"
	case FRINTZS:
		return "FRINTZS"
	case FSQRTD:
		return "FSQRTD"
	case FSQRTS:
		return "FSQRTS"
	case FSUBD:
		return "FSUBD"
	case FSUBS:
		return "FSUBS"
	case LSL:
		return "LSL"
	case LSLW:
		return "LSLW"
	case LSR:
		return "LSR"
	case LSRW:
		return "LSRW"
	case MOVB:
		return "MOVB"
	case MOVBU:
		return "MOVBU"
	case MOVD:
		return "MOVD"
	case MOVH:
		return "MOVH"
	case MOVHU:
		return "MOVHU"
	case MOVW:
		return "MOVW"
	case MOVWU:
		return "MOVWU"
	case MRS:
		return "MRS"
	case MSR:
		return "MSR"
	case MSUB:
		return "MSUB"
	case MSUBW:
		return "MSUBW"
	case MUL:
		return "MUL"
	case MULW:
		return "MULW"
	case NEG:
		return "NEG"
	case NEGW:
		return "NEGW"
	case ORR:
		return "ORR"
	case ORRW:
		return "ORRW"
	case RBIT:
		return "RBIT"
	case RBITW:
		return "RBITW"
	case RNG:
		return "RNG"
	case ROR:
		return "ROR"
	case RORW:
		return "RORW"
	case SCVTFD:
		return "SCVTFD"
	case SCVTFS:
		return "SCVTFS"
	case SCVTFWD:
		return "SCVTFWD"
	case SCVTFWS:
		return "SCVTFWS"
	case SDIV:
		return "SDIV"
	case SDIVW:
		return "SDIVW"
	case SUB:
		return "SUB"
	case SUBS:
		return "SUBS"
	case SUBW:
		return "SUBW"
	case SXTB:
		return "SXTB"
	case SXTBW:
		return "SXTBW"
	case SXTH:
		return "SXTH"
	case SXTHW:
		return "SXTHW"
	case SXTW:
		return "SXTW"
	case UCVTFD:
		return "UCVTFD"
	case UCVTFS:
		return "UCVTFS"
	case UCVTFWD:
		return "UCVTFWD"
	case UCVTFWS:
		return "UCVTFWS"
	case UDIV:
		return "UDIV"
	case UDIVW:
		return "UDIVW"
	case UXTW:
		return "UXTW"
	case VBIT:
		return "VBIT"
	case VCNT:
		return "VCNT"
	case VUADDLV:
		return "VUADDLV"
	case VMOV:
		return "VMOV"
	case VST1:
		return "VST1"
	case VLD1:
		return "VLD1"
	case VADD:
		return "VADD"
	case VFADDS:
		return "VFADDS"
	case VFADDD:
		return "VFADDD"
	}
	return "UNKNOWN"
}
