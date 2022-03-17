package arm64

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

type assemblerGoAsmImpl struct {
	*asm.GolangAsmBaseAssembler
	temporaryRegister asm.Register
}

var _ Assembler = &assemblerGoAsmImpl{}

func newGolangAsmAssembler(temporaryRegister asm.Register) (*assemblerGoAsmImpl, error) {
	g, err := asm.NewGolangAsmBaseAssembler()
	return &assemblerGoAsmImpl{GolangAsmBaseAssembler: g, temporaryRegister: temporaryRegister}, err
}

func (a *assemblerGoAsmImpl) CompileConstToRegisterInstruction(instruction asm.Instruction, constValue int64, destinationReg asm.Register) asm.Node {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	if constValue == 0 {
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = arm64.REGZERO
	} else {
		inst.From.Type = obj.TYPE_CONST
		// Note: in raw arm64 assembly, immediates larger than 16-bits
		// are not supported, but the assembler takes care of this and
		// emits corresponding (at most) 4-instructions to load such large constants.
		inst.From.Offset = constValue
	}

	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(inst)
	return nil
}

func (a *assemblerGoAsmImpl) CompileMemoryToRegisterInstruction(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register) {
	if sourceOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		a.CompileConstToRegisterInstruction(MOVD, sourceOffsetConst, a.temporaryRegister)
		a.CompileMemoryWithRegisterOffsetToRegisterInstruction(instruction, sourceBaseReg, a.temporaryRegister, destinationReg)
	} else {
		inst := a.NewProg()
		inst.As = castAsGolangAsmInstruction[instruction]
		inst.From.Type = obj.TYPE_MEM
		inst.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
		inst.From.Offset = sourceOffsetConst
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = castAsGolangAsmRegister[destinationReg]
		a.AddInstruction(inst)
	}
}

func (a *assemblerGoAsmImpl) CompileMemoryWithRegisterOffsetToRegisterInstruction(instruction asm.Instruction, sourceBaseReg, sourceOffsetReg, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	inst.From.Index = castAsGolangAsmRegister[sourceOffsetReg]
	inst.From.Scale = 1
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileRegisterToMemoryInstruction(instruction asm.Instruction, sourceReg asm.Register, destinationBaseReg asm.Register, destinationOffsetConst int64) {
	if destinationOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but we cannot track its temporary register. Therefore, we avoid directly emitting memory load with large offsets:
		// load the constant manually to "our" temporary register, then emit the load with it.
		a.CompileConstToRegisterInstruction(MOVD, destinationOffsetConst, a.temporaryRegister)
		a.CompileRegisterToMemoryWithRegisterOffsetInstruction(instruction, sourceReg, destinationBaseReg, a.temporaryRegister)
	} else {
		inst := a.NewProg()
		inst.As = castAsGolangAsmInstruction[instruction]
		inst.To.Type = obj.TYPE_MEM
		inst.To.Reg = castAsGolangAsmRegister[destinationBaseReg]
		inst.To.Offset = destinationOffsetConst
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = castAsGolangAsmRegister[sourceReg]
		a.AddInstruction(inst)
	}
}

func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithRegisterOffsetInstruction(instruction asm.Instruction, sourceReg, destinationBaseReg, destinationOffsetReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = castAsGolangAsmRegister[destinationBaseReg]
	inst.To.Index = castAsGolangAsmRegister[destinationOffsetReg]
	inst.To.Scale = 1
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[sourceReg]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileRegisterToRegisterInstruction(instruction asm.Instruction, from, to asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[to]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[from]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileTwoRegistersToRegisterInstruction(instruction asm.Instruction, src1, src2, destination asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destination]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileTwoRegistersInstruction(instruction asm.Instruction, src1, src2, dst1, dst2 asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[dst1]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	inst.RestArgs = append(inst.RestArgs, obj.Addr{Type: obj.TYPE_REG, Reg: castAsGolangAsmRegister[dst2]})
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileTwoRegistersToNoneInstruction(instruction asm.Instruction, src1, src2 asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero in anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileRegisterAndConstSourceToNoneInstruction(instruction asm.Instruction, src asm.Register, srcConst int64) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero in anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_CONST
	inst.From.Offset = srcConst
	inst.Reg = castAsGolangAsmRegister[src]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileBranchInstruction(instruction asm.Instruction) asm.Node {
	br := a.NewProg()
	br.As = castAsGolangAsmInstruction[instruction]
	br.To.Type = obj.TYPE_BRANCH
	a.AddInstruction(br)
	return asm.NewGolangAsmNode(br)
}

func (a *assemblerGoAsmImpl) CompileUnconditionalBranchToAddressOnMemory(memoryLocationReg asm.Register) {
	br := a.NewProg()
	br.As = obj.AJMP
	br.To.Type = obj.TYPE_MEM
	br.To.Reg = castAsGolangAsmRegister[memoryLocationReg]
	a.AddInstruction(br)
}

func (a *assemblerGoAsmImpl) CompileReturn(returnAddressReg asm.Register) {
	ret := a.NewProg()
	ret.As = obj.ARET
	ret.To.Type = obj.TYPE_REG
	ret.To.Reg = castAsGolangAsmRegister[returnAddressReg]
	a.AddInstruction(ret)
}

func (a *assemblerGoAsmImpl) CompileStandAloneInstruction(instruction asm.Instruction) asm.Node {
	prog := a.NewProg()
	prog.As = castAsGolangAsmInstruction[instruction]
	a.AddInstruction(prog)
	return nil
}

func (a *assemblerGoAsmImpl) CompileAddInstructionWithLeftShiftedRegister(shiftedSourceReg asm.Register, shiftNum int64, srcReg, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = arm64.AADD
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	// See https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L120-L131
	inst.From.Type = obj.TYPE_SHIFT
	inst.From.Offset = (int64(shiftedSourceReg)&31)<<16 | 0<<22 | (shiftNum&63)<<10
	inst.Reg = castAsGolangAsmRegister[srcReg]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileReadInstructionAddress(beforeTargetInst asm.Instruction, destinationReg asm.Register) {
	// Emit ADR instruction to read the specified instruction's absolute address.
	// Note: we cannot emit the "ADR REG, $(target's offset from here)" due to the
	// incapability of the assembler. Instead, we emit "ADR REG, ." meaning that
	// "reading the current program counter" = "reading the absolute address of this ADR instruction".
	// And then, after compilation phase, we directly edit the native code slice so that
	// it can properly read the target instruction's absolute address.
	readAddress := a.NewProg()
	readAddress.As = arm64.AADR
	readAddress.From.Type = obj.TYPE_BRANCH
	readAddress.To.Type = obj.TYPE_REG
	readAddress.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(readAddress)

	// Setup the callback to modify the instruction bytes after compilation.
	// Note: this is the closure over readAddress (*obj.Prog).
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Find the target instruction.
		target := readAddress
		beforeTarget := castAsGolangAsmInstruction[beforeTargetInst]
		for target != nil {
			if target.As == beforeTarget {
				// At this point, target is the instruction right before the target instruction.
				// Thus, advance one more time to make target the target instruction.
				target = target.Link
				break
			}
			target = target.Link
		}

		if target == nil {
			return fmt.Errorf("BUG: target instruction not found for read instruction address")
		}

		offset := target.Pc - readAddress.Pc
		if offset > math.MaxUint8 {
			// We could support up to 20-bit integer, but byte should be enough for our impl.
			// If the necessity comes up, we could fix the below to support larger offsets.
			return fmt.Errorf("BUG: too large offset for read")
		}

		// Now ready to write an offset byte.
		v := byte(offset)
		// arm64 has 4-bytes = 32-bit fixed-length instruction.
		adrInstructionBytes := code[readAddress.Pc : readAddress.Pc+4]
		// According to the binary format of ADR instruction in arm64:
		// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/ADR--Form-PC-relative-address-?lang=en
		//
		// The 0 to 1 bits live on 29 to 30 bits of the instruction.
		adrInstructionBytes[3] |= (v & 0b00000011) << 5
		// The 2 to 4 bits live on 5 to 7 bits of the instruction.
		adrInstructionBytes[0] |= (v & 0b00011100) << 3
		// The 5 to 7 bits live on 8 to 10 bits of the instruction.
		adrInstructionBytes[1] |= (v & 0b11100000) >> 5
		return nil
	})
}

func (a *assemblerGoAsmImpl) BuildJumpTable(table []byte, labelInitialInstructions []asm.Node) {
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Build the offset table for each target including default one.
		base := labelInitialInstructions[0].Pc() // This corresponds to the L0's address in the example.
		for i, nop := range labelInitialInstructions {
			if uint64(nop.Pc())-uint64(base) >= math.MaxUint32 {
				// TODO: this happens when users try loading an extremely large webassembly binary
				// which contains a br_table statement with approximately 4294967296 (2^32) targets.
				// We would like to support that binary, but realistically speaking, that kind of binary
				// could result in more than ten giga bytes of native JITed code where we have to care about
				// huge stacks whose height might exceed 32-bit range, and such huge stack doesn't work with the
				// current implementation.
				return fmt.Errorf("too large br_table")
			}
			// We store the offset from the beginning of the L0's initial instruction.
			binary.LittleEndian.PutUint32(table[i*4:(i+1)*4], uint32(nop.Pc())-uint32(base))
		}
		return nil
	})
}

func (a *assemblerGoAsmImpl) CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = arm64.ACSET
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmConditionalRegister[cond]
	a.AddInstruction(inst)
}

var castAsGolangAsmConditionalRegister = [...]int16{
	COND_EQ: arm64.COND_EQ,
	COND_NE: arm64.COND_NE,
	COND_HS: arm64.COND_HS,
	COND_LO: arm64.COND_LO,
	COND_MI: arm64.COND_MI,
	COND_PL: arm64.COND_PL,
	COND_VS: arm64.COND_VS,
	COND_VC: arm64.COND_VC,
	COND_HI: arm64.COND_HI,
	COND_LS: arm64.COND_LS,
	COND_GE: arm64.COND_GE,
	COND_LT: arm64.COND_LT,
	COND_GT: arm64.COND_GT,
	COND_LE: arm64.COND_LE,
	COND_AL: arm64.COND_AL,
	COND_NV: arm64.COND_NV,
}

var castAsGolangAsmRegister = [...]int16{
	REG_R0:  arm64.REG_R0,
	REG_R1:  arm64.REG_R1,
	REG_R2:  arm64.REG_R2,
	REG_R3:  arm64.REG_R3,
	REG_R4:  arm64.REG_R4,
	REG_R5:  arm64.REG_R5,
	REG_R6:  arm64.REG_R6,
	REG_R7:  arm64.REG_R7,
	REG_R8:  arm64.REG_R8,
	REG_R9:  arm64.REG_R9,
	REG_R10: arm64.REG_R10,
	REG_R11: arm64.REG_R11,
	REG_R12: arm64.REG_R12,
	REG_R13: arm64.REG_R13,
	REG_R14: arm64.REG_R14,
	REG_R15: arm64.REG_R15,
	REG_R16: arm64.REG_R16,
	REG_R17: arm64.REG_R17,
	REG_R18: arm64.REG_R18,
	REG_R19: arm64.REG_R19,
	REG_R20: arm64.REG_R20,
	REG_R21: arm64.REG_R21,
	REG_R22: arm64.REG_R22,
	REG_R23: arm64.REG_R23,
	REG_R24: arm64.REG_R24,
	REG_R25: arm64.REG_R25,
	REG_R26: arm64.REG_R26,
	REG_R27: arm64.REG_R27,
	REG_R28: arm64.REG_R28,
	REG_R29: arm64.REG_R29,
	REG_R30: arm64.REG_R30,
	REGZERO: arm64.REGZERO,
	REG_F0:  arm64.REG_F0,
	REG_F1:  arm64.REG_F1,
	REG_F2:  arm64.REG_F2,
	REG_F3:  arm64.REG_F3,
	REG_F4:  arm64.REG_F4,
	REG_F5:  arm64.REG_F5,
	REG_F6:  arm64.REG_F6,
	REG_F7:  arm64.REG_F7,
	REG_F8:  arm64.REG_F8,
	REG_F9:  arm64.REG_F9,
	REG_F10: arm64.REG_F10,
	REG_F11: arm64.REG_F11,
	REG_F12: arm64.REG_F12,
	REG_F13: arm64.REG_F13,
	REG_F14: arm64.REG_F14,
	REG_F15: arm64.REG_F15,
	REG_F16: arm64.REG_F16,
	REG_F17: arm64.REG_F17,
	REG_F18: arm64.REG_F18,
	REG_F19: arm64.REG_F19,
	REG_F20: arm64.REG_F20,
	REG_F21: arm64.REG_F21,
	REG_F22: arm64.REG_F22,
	REG_F23: arm64.REG_F23,
	REG_F24: arm64.REG_F24,
	REG_F25: arm64.REG_F25,
	REG_F26: arm64.REG_F26,
	REG_F27: arm64.REG_F27,
	REG_F28: arm64.REG_F28,
	REG_F29: arm64.REG_F29,
	REG_F30: arm64.REG_F30,
	REG_F31: arm64.REG_F31,
	REG_V0:  arm64.REG_V0,
	REG_V1:  arm64.REG_V1,
	REG_V2:  arm64.REG_V2,
	REG_V3:  arm64.REG_V3,
	REG_V4:  arm64.REG_V4,
	REG_V5:  arm64.REG_V5,
	REG_V6:  arm64.REG_V6,
	REG_V7:  arm64.REG_V7,
	REG_V8:  arm64.REG_V8,
	REG_V9:  arm64.REG_V9,
	REG_V10: arm64.REG_V10,
	REG_V11: arm64.REG_V11,
	REG_V12: arm64.REG_V12,
	REG_V13: arm64.REG_V13,
	REG_V14: arm64.REG_V14,
	REG_V15: arm64.REG_V15,
	REG_V16: arm64.REG_V16,
	REG_V17: arm64.REG_V17,
	REG_V18: arm64.REG_V18,
	REG_V19: arm64.REG_V19,
	REG_V20: arm64.REG_V20,
	REG_V21: arm64.REG_V21,
	REG_V22: arm64.REG_V22,
	REG_V23: arm64.REG_V23,
	REG_V24: arm64.REG_V24,
	REG_V25: arm64.REG_V25,
	REG_V26: arm64.REG_V26,
	REG_V27: arm64.REG_V27,
	REG_V28: arm64.REG_V28,
	REG_V29: arm64.REG_V29,
	REG_V30: arm64.REG_V30,
	REG_V31: arm64.REG_V31,
}

var castAsGolangAsmInstruction = [...]obj.As{
	NOP:      obj.ANOP,
	RET:      obj.ANOP,
	ADD:      arm64.AADD,
	ADDW:     arm64.AADDW,
	ADR:      arm64.AADR,
	AND:      arm64.AAND,
	ANDW:     arm64.AANDW,
	ASR:      arm64.AASR,
	ASRW:     arm64.AASRW,
	B:        arm64.AB,
	BEQ:      arm64.ABEQ,
	BGE:      arm64.ABGE,
	BGT:      arm64.ABGT,
	BHI:      arm64.ABHI,
	BHS:      arm64.ABHS,
	BLE:      arm64.ABLE,
	BLO:      arm64.ABLO,
	BLS:      arm64.ABLS,
	BLT:      arm64.ABLT,
	BMI:      arm64.ABMI,
	BNE:      arm64.ABNE,
	BVS:      arm64.ABVS,
	CLZ:      arm64.ACLZ,
	CLZW:     arm64.ACLZW,
	CMP:      arm64.ACMP,
	CMPW:     arm64.ACMPW,
	CSET:     arm64.ACSET,
	EOR:      arm64.AEOR,
	EORW:     arm64.AEORW,
	FABSD:    arm64.AFABSD,
	FABSS:    arm64.AFABSS,
	FADDD:    arm64.AFADDD,
	FADDS:    arm64.AFADDS,
	FCMPD:    arm64.AFCMPD,
	FCMPS:    arm64.AFCMPS,
	FCVTDS:   arm64.AFCVTDS,
	FCVTSD:   arm64.AFCVTSD,
	FCVTZSD:  arm64.AFCVTZSD,
	FCVTZSDW: arm64.AFCVTZSDW,
	FCVTZSS:  arm64.AFCVTZSS,
	FCVTZSSW: arm64.AFCVTZSSW,
	FCVTZUD:  arm64.AFCVTZUD,
	FCVTZUDW: arm64.AFCVTZUDW,
	FCVTZUS:  arm64.AFCVTZUS,
	FCVTZUSW: arm64.AFCVTZUSW,
	FDIVD:    arm64.AFDIVD,
	FDIVS:    arm64.AFDIVS,
	FMAXD:    arm64.AFMAXD,
	FMAXS:    arm64.AFMAXS,
	FMIND:    arm64.AFMIND,
	FMINS:    arm64.AFMINS,
	FMOVD:    arm64.AFMOVD,
	FMOVS:    arm64.AFMOVS,
	FMULD:    arm64.AFMULD,
	FMULS:    arm64.AFMULS,
	FNEGD:    arm64.AFNEGD,
	FNEGS:    arm64.AFNEGS,
	FRINTMD:  arm64.AFRINTMD,
	FRINTMS:  arm64.AFRINTMS,
	FRINTND:  arm64.AFRINTND,
	FRINTNS:  arm64.AFRINTNS,
	FRINTPD:  arm64.AFRINTPD,
	FRINTPS:  arm64.AFRINTPS,
	FRINTZD:  arm64.AFRINTZD,
	FRINTZS:  arm64.AFRINTZS,
	FSQRTD:   arm64.AFSQRTD,
	FSQRTS:   arm64.AFSQRTS,
	FSUBD:    arm64.AFSUBD,
	FSUBS:    arm64.AFSUBS,
	LSL:      arm64.ALSL,
	LSLW:     arm64.ALSLW,
	LSR:      arm64.ALSR,
	LSRW:     arm64.ALSRW,
	MOVB:     arm64.AMOVB,
	MOVBU:    arm64.AMOVBU,
	MOVD:     arm64.AMOVD,
	MOVH:     arm64.AMOVH,
	MOVHU:    arm64.AMOVHU,
	MOVW:     arm64.AMOVW,
	MOVWU:    arm64.AMOVWU,
	MRS:      arm64.AMRS,
	MSR:      arm64.AMSR,
	MSUB:     arm64.AMSUB,
	MSUBW:    arm64.AMSUBW,
	MUL:      arm64.AMUL,
	MULW:     arm64.AMULW,
	NEG:      arm64.ANEG,
	NEGW:     arm64.ANEGW,
	ORR:      arm64.AORR,
	ORRW:     arm64.AORRW,
	RBIT:     arm64.ARBIT,
	RBITW:    arm64.ARBITW,
	// RNG:      arm64.ARNG, TODO!!!!!!!
	ROR:     arm64.AROR,
	RORW:    arm64.ARORW,
	SCVTFD:  arm64.ASCVTFD,
	SCVTFS:  arm64.ASCVTFS,
	SCVTFWD: arm64.ASCVTFWD,
	SCVTFWS: arm64.ASCVTFWS,
	SDIV:    arm64.ASDIV,
	SDIVW:   arm64.ASDIVW,
	SUB:     arm64.ASUB,
	SUBS:    arm64.ASUBS,
	SUBW:    arm64.ASUBW,
	SXTB:    arm64.ASXTB,
	SXTBW:   arm64.ASXTBW,
	SXTH:    arm64.ASXTH,
	SXTHW:   arm64.ASXTHW,
	SXTW:    arm64.ASXTW,
	UCVTFD:  arm64.AUCVTFD,
	UCVTFS:  arm64.AUCVTFS,
	UCVTFWD: arm64.AUCVTFWD,
	UCVTFWS: arm64.AUCVTFWS,
	UDIV:    arm64.AUDIV,
	UDIVW:   arm64.AUDIVW,
	UXTW:    arm64.AUXTW,
	VBIT:    arm64.AVBIT,
	VCNT:    arm64.AVCNT,
	VUADDLV: arm64.AVUADDLV,
}
