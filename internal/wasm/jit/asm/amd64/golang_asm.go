package amd64

import (
	"encoding/binary"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

// assemblerGoAsmImpl implements Assembler for golang-asm library.
type assemblerGoAsmImpl struct {
	*asm.GolangAsmBaseAssembler
}

func newGolangAsmAssembler() (*assemblerGoAsmImpl, error) {
	g, err := asm.NewGolangAsmBaseAssembler()
	return &assemblerGoAsmImpl{g}, err
}

// CompileStandAlone implements Assembler.CompileStandAlone.
func (a *assemblerGoAsmImpl) CompileStandAlone(inst asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileRegisterToRegister implements Assembler.CompileRegisterToRegister.
func (a *assemblerGoAsmImpl) CompileRegisterToRegister(inst asm.Instruction, from, to asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[from]
	a.AddInstruction(p)
}

// CompileMemoryWithIndexToRegister implements Assembler.CompileMemoryWithIndexToRegister.
func (a *assemblerGoAsmImpl) CompileMemoryWithIndexToRegister(inst asm.Instruction,
	sourceBaseReg asm.Register, sourceOffsetConst int64, sourceIndexReg asm.Register, sourceScale int16, destinationReg asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationReg]
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	p.From.Offset = sourceOffsetConst
	p.From.Index = castAsGolangAsmRegister[sourceIndexReg]
	p.From.Scale = sourceScale
	a.AddInstruction(p)
}

// CompileRegisterToMemoryWithIndex implements Assembler.CompileRegisterToMemoryWithIndex.
func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithIndex(inst asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndexReg asm.Register, dstScale int16) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[srcReg]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[dstBaseReg]
	p.To.Offset = dstOffsetConst
	p.To.Index = castAsGolangAsmRegister[dstIndexReg]
	p.To.Scale = dstScale
	a.AddInstruction(p)
}

// CompileRegisterToMemory implements Assembler.CompileRegisterToMemory.
func (a *assemblerGoAsmImpl) CompileRegisterToMemory(inst asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[destinationBaseRegister]
	p.To.Offset = destinationOffsetConst
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[sourceRegister]
	a.AddInstruction(p)
}

// CompileConstToRegister implements Assembler.CompileConstToRegister.
func (a *assemblerGoAsmImpl) CompileConstToRegister(inst asm.Instruction, constValue int64, destinationRegister asm.Register) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationRegister]
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileRegisterToConst implements Assembler.CompileRegisterToConst.
func (a *assemblerGoAsmImpl) CompileRegisterToConst(inst asm.Instruction, srcRegister asm.Register, constValue int64) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[srcRegister]
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileRegisterToNone implements Assembler.CompileRegisterToNone.
func (a *assemblerGoAsmImpl) CompileRegisterToNone(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[register]
	p.To.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToRegister implements Assembler.CompileNoneToRegister.
func (a *assemblerGoAsmImpl) CompileNoneToRegister(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[register]
	p.From.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToMemory implements Assembler.CompileNoneToMemory.
func (a *assemblerGoAsmImpl) CompileNoneToMemory(inst asm.Instruction, baseReg asm.Register, offset int64) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	p.From.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileConstToMemory implements Assembler.CompileConstToMemory.
func (a *assemblerGoAsmImpl) CompileConstToMemory(inst asm.Instruction, constValue int64, baseReg asm.Register, offset int64) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileMemoryToRegister implements AssemblerBase.CompileMemoryToRegister.
func (a *assemblerGoAsmImpl) CompileMemoryToRegister(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	p.From.Offset = sourceOffsetConst
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(p)
}

// CompileMemoryToConst implements Assembler.CompileMemoryToConst.
func (a *assemblerGoAsmImpl) CompileMemoryToConst(inst asm.Instruction, baseReg asm.Register, offset int64, constValue int64) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[baseReg]
	p.From.Offset = offset
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileJump implements Assembler.CompileJump.
func (a *assemblerGoAsmImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_BRANCH
	a.AddInstruction(p)
	return asm.NewGolangAsmNode(p)
}

// CompileJumpToRegister implements Assembler.CompileJumpToRegister.
func (a *assemblerGoAsmImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[reg]
	a.AddInstruction(p)
}

// CompileJumpToMemory implements Assembler.CompileJumpToMemory.
func (a *assemblerGoAsmImpl) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register, offset int64) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	a.AddInstruction(p)
}

// CompileModeRegisterToRegister implements Assembler.CompileModeRegisterToRegister.
func (a *assemblerGoAsmImpl) CompileModeRegisterToRegister(inst asm.Instruction, from, to asm.Register, mode int64) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = mode
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.RestArgs = append(p.RestArgs,
		obj.Addr{Reg: castAsGolangAsmRegister[from], Type: obj.TYPE_REG})
	a.AddInstruction(p)
}

// CompileReadInstructionAddress implements Assembler.CompileReadInstructionAddress.
func (a *assemblerGoAsmImpl) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	// Emit the instruction in the form of "LEA destination [RIP + offset]".
	readInstructionAddress := a.NewProg()
	readInstructionAddress.As = x86.ALEAQ
	readInstructionAddress.To.Reg = castAsGolangAsmRegister[destinationRegister]
	readInstructionAddress.To.Type = obj.TYPE_REG
	readInstructionAddress.From.Type = obj.TYPE_MEM
	// We use place holder here as we don't yet know at this point the offset of the first instruction
	// after return instruction.
	readInstructionAddress.From.Offset = 0xffff
	// Since the assembler cannot directly emit "LEA destination [RIP + offset]", we use the some hack here:
	// We intentionally use x86.REG_BP here so that the resulting instruction sequence becomes
	// exactly the same as "LEA destination [RIP + offset]" except the most significant bit of the third byte.
	// We do the rewrite in onGenerateCallbacks which is invoked after the assembler emitted the code.
	readInstructionAddress.From.Reg = x86.REG_BP
	a.AddInstruction(readInstructionAddress)

	a.AddOnGenerateCallBack(func(code []byte) error {
		// Advance readInstructionAddress to the next one (.Link) in order to get the instruction
		// right after LEA because RIP points to that next instruction in LEA instruction.
		base := readInstructionAddress.Link

		// Find the address acquisition target instruction.
		target := base
		beforeTargetInst := castAsGolangAsmInstruction[beforeAcquisitionTargetInstruction]
		for target != nil {
			// Advance until we have the target.As has the given instruction kind.
			target = target.Link
			if target.As == beforeTargetInst {
				// At this point, target is the instruction right before the target instruction.
				// Thus, advance one more time to make target the target instruction.
				target = target.Link
				break
			}
		}

		if target == nil {
			return fmt.Errorf("target instruction not found for read instruction address")
		}

		// Now we can calculate the "offset" in the LEA instruction.
		offset := uint32(target.Pc) - uint32(base.Pc)

		// Replace the placeholder bytes by the actual offset.
		binary.LittleEndian.PutUint32(code[readInstructionAddress.Pc+3:], offset)

		// See the comment at readInstructionAddress.From.Reg above. Here we drop the most significant bit of the third byte of the LEA instruction.
		code[readInstructionAddress.Pc+2] &= 0b01111111
		return nil
	})
}

// castAsGolangAsmRegister maps the registers to golang-asm specific register values.
var castAsGolangAsmRegister = [...]int16{
	REG_AX:  x86.REG_AX,
	REG_CX:  x86.REG_CX,
	REG_DX:  x86.REG_DX,
	REG_BX:  x86.REG_BX,
	REG_SP:  x86.REG_SP,
	REG_BP:  x86.REG_BP,
	REG_SI:  x86.REG_SI,
	REG_DI:  x86.REG_DI,
	REG_R8:  x86.REG_R8,
	REG_R9:  x86.REG_R9,
	REG_R10: x86.REG_R10,
	REG_R11: x86.REG_R11,
	REG_R12: x86.REG_R12,
	REG_R13: x86.REG_R13,
	REG_R14: x86.REG_R14,
	REG_R15: x86.REG_R15,
	REG_X0:  x86.REG_X0,
	REG_X1:  x86.REG_X1,
	REG_X2:  x86.REG_X2,
	REG_X3:  x86.REG_X3,
	REG_X4:  x86.REG_X4,
	REG_X5:  x86.REG_X5,
	REG_X6:  x86.REG_X6,
	REG_X7:  x86.REG_X7,
	REG_X8:  x86.REG_X8,
	REG_X9:  x86.REG_X9,
	REG_X10: x86.REG_X10,
	REG_X11: x86.REG_X11,
	REG_X12: x86.REG_X12,
	REG_X13: x86.REG_X13,
	REG_X14: x86.REG_X14,
	REG_X15: x86.REG_X15,
	REG_X16: x86.REG_X16,
	REG_X17: x86.REG_X17,
	REG_X18: x86.REG_X18,
	REG_X19: x86.REG_X19,
	REG_X20: x86.REG_X20,
	REG_X21: x86.REG_X21,
	REG_X22: x86.REG_X22,
	REG_X23: x86.REG_X23,
	REG_X24: x86.REG_X24,
	REG_X25: x86.REG_X25,
	REG_X26: x86.REG_X26,
	REG_X27: x86.REG_X27,
	REG_X28: x86.REG_X28,
	REG_X29: x86.REG_X29,
	REG_X30: x86.REG_X30,
	REG_X31: x86.REG_X31,
}

// castAsGolangAsmRegister maps the instructions to golang-asm specific instruction values.
var castAsGolangAsmInstruction = [...]obj.As{
	NOP:       obj.ANOP,
	RET:       obj.ARET,
	JMP:       obj.AJMP,
	UD2:       x86.AUD2,
	ADDL:      x86.AADDL,
	ADDQ:      x86.AADDQ,
	ADDSD:     x86.AADDSD,
	ADDSS:     x86.AADDSS,
	ANDL:      x86.AANDL,
	ANDPD:     x86.AANDPD,
	ANDPS:     x86.AANDPS,
	ANDQ:      x86.AANDQ,
	BSRL:      x86.ABSRL,
	BSRQ:      x86.ABSRQ,
	CDQ:       x86.ACDQ,
	CMOVQCS:   x86.ACMOVQCS,
	CMPL:      x86.ACMPL,
	CMPQ:      x86.ACMPQ,
	COMISD:    x86.ACOMISD,
	COMISS:    x86.ACOMISS,
	CQO:       x86.ACQO,
	CVTSD2SS:  x86.ACVTSD2SS,
	CVTSL2SD:  x86.ACVTSL2SD,
	CVTSL2SS:  x86.ACVTSL2SS,
	CVTSQ2SD:  x86.ACVTSQ2SD,
	CVTSQ2SS:  x86.ACVTSQ2SS,
	CVTSS2SD:  x86.ACVTSS2SD,
	CVTTSD2SL: x86.ACVTTSD2SL,
	CVTTSD2SQ: x86.ACVTTSD2SQ,
	CVTTSS2SL: x86.ACVTTSS2SL,
	CVTTSS2SQ: x86.ACVTTSS2SQ,
	DECQ:      x86.ADECQ,
	DIVL:      x86.ADIVL,
	DIVQ:      x86.ADIVQ,
	DIVSD:     x86.ADIVSD,
	DIVSS:     x86.ADIVSS,
	IDIVL:     x86.AIDIVL,
	IDIVQ:     x86.AIDIVQ,
	INCQ:      x86.AINCQ,
	JCC:       x86.AJCC,
	JCS:       x86.AJCS,
	JEQ:       x86.AJEQ,
	JGE:       x86.AJGE,
	JGT:       x86.AJGT,
	JHI:       x86.AJHI,
	JLE:       x86.AJLE,
	JLS:       x86.AJLS,
	JLT:       x86.AJLT,
	JMI:       x86.AJMI,
	JNE:       x86.AJNE,
	JPC:       x86.AJPC,
	JPL:       x86.AJPL,
	JPS:       x86.AJPS,
	LEAQ:      x86.ALEAQ,
	LZCNTL:    x86.ALZCNTL,
	LZCNTQ:    x86.ALZCNTQ,
	MAXSD:     x86.AMAXSD,
	MAXSS:     x86.AMAXSS,
	MINSD:     x86.AMINSD,
	MINSS:     x86.AMINSS,
	MOVB:      x86.AMOVB,
	MOVBLSX:   x86.AMOVBLSX,
	MOVBLZX:   x86.AMOVBLZX,
	MOVBQSX:   x86.AMOVBQSX,
	MOVBQZX:   x86.AMOVBQZX,
	MOVL:      x86.AMOVL,
	MOVLQSX:   x86.AMOVLQSX,
	MOVLQZX:   x86.AMOVLQZX,
	MOVQ:      x86.AMOVQ,
	MOVW:      x86.AMOVW,
	MOVWLSX:   x86.AMOVWLSX,
	MOVWLZX:   x86.AMOVWLZX,
	MOVWQSX:   x86.AMOVWQSX,
	MOVWQZX:   x86.AMOVWQZX,
	MULL:      x86.AMULL,
	MULQ:      x86.AMULQ,
	MULSD:     x86.AMULSD,
	MULSS:     x86.AMULSS,
	ORL:       x86.AORL,
	ORPD:      x86.AORPD,
	ORPS:      x86.AORPS,
	ORQ:       x86.AORQ,
	POPCNTL:   x86.APOPCNTL,
	POPCNTQ:   x86.APOPCNTQ,
	PSLLL:     x86.APSLLL,
	PSLLQ:     x86.APSLLQ,
	PSRLL:     x86.APSRLL,
	PSRLQ:     x86.APSRLQ,
	ROLL:      x86.AROLL,
	ROLQ:      x86.AROLQ,
	RORL:      x86.ARORL,
	RORQ:      x86.ARORQ,
	ROUNDSD:   x86.AROUNDSD,
	ROUNDSS:   x86.AROUNDSS,
	SARL:      x86.ASARL,
	SARQ:      x86.ASARQ,
	SETCC:     x86.ASETCC,
	SETCS:     x86.ASETCS,
	SETEQ:     x86.ASETEQ,
	SETGE:     x86.ASETGE,
	SETGT:     x86.ASETGT,
	SETHI:     x86.ASETHI,
	SETLE:     x86.ASETLE,
	SETLS:     x86.ASETLS,
	SETLT:     x86.ASETLT,
	SETMI:     x86.ASETMI,
	SETNE:     x86.ASETNE,
	SETPC:     x86.ASETPC,
	SETPL:     x86.ASETPL,
	SETPS:     x86.ASETPS,
	SHLL:      x86.ASHLL,
	SHLQ:      x86.ASHLQ,
	SHRL:      x86.ASHRL,
	SHRQ:      x86.ASHRQ,
	SQRTSD:    x86.ASQRTSD,
	SQRTSS:    x86.ASQRTSS,
	SUBL:      x86.ASUBL,
	SUBQ:      x86.ASUBQ,
	SUBSD:     x86.ASUBSD,
	SUBSS:     x86.ASUBSS,
	TESTL:     x86.ATESTL,
	TESTQ:     x86.ATESTQ,
	TZCNTL:    x86.ATZCNTL,
	TZCNTQ:    x86.ATZCNTQ,
	UCOMISD:   x86.AUCOMISD,
	UCOMISS:   x86.AUCOMISS,
	XORL:      x86.AXORL,
	XORPD:     x86.AXORPD,
	XORPS:     x86.AXORPS,
	XORQ:      x86.AXORQ,
}
