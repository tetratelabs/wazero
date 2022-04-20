package amd64_debug

import (
	"encoding/binary"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/heeus/hwazero/internal/asm"
	asm_amd64 "github.com/heeus/hwazero/internal/asm/amd64"
	"github.com/heeus/hwazero/internal/integration_test/asm/golang_asm"
)

// assemblerGoAsmImpl implements asm_amd64.Assembler for golang-asm library.
type assemblerGoAsmImpl struct {
	*golang_asm.GolangAsmBaseAssembler
}

func newGolangAsmAssembler() (*assemblerGoAsmImpl, error) {
	g, err := golang_asm.NewGolangAsmBaseAssembler("amd64")
	return &assemblerGoAsmImpl{g}, err
}

// CompileStandAlone implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileStandAlone(inst asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileRegisterToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToRegister(inst asm.Instruction, from, to asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[from]
	a.AddInstruction(p)
}

// CompileMemoryWithIndexToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryWithIndexToRegister(
	inst asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	sourceIndexReg asm.Register,
	sourceScale int16,
	destinationReg asm.Register,
) {
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

// CompileRegisterToMemoryWithIndex implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithIndex(
	inst asm.Instruction,
	srcReg, dstBaseReg asm.Register,
	dstOffsetConst asm.ConstantValue,
	dstIndexReg asm.Register,
	dstScale int16,
) {
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

// CompileRegisterToMemory implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToMemory(
	inst asm.Instruction,
	sourceRegister, destinationBaseRegister asm.Register,
	destinationOffsetConst asm.ConstantValue,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[destinationBaseRegister]
	p.To.Offset = destinationOffsetConst
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[sourceRegister]
	a.AddInstruction(p)
}

// CompileConstToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileConstToRegister(
	inst asm.Instruction,
	constValue asm.ConstantValue,
	destinationRegister asm.Register,
) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationRegister]
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileRegisterToConst implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToConst(
	inst asm.Instruction,
	srcRegister asm.Register,
	constValue asm.ConstantValue,
) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[srcRegister]
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileRegisterToNone implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToNone(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[register]
	p.To.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileNoneToRegister(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[register]
	p.From.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToMemory implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileNoneToMemory(
	inst asm.Instruction,
	baseReg asm.Register,
	offset asm.ConstantValue,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	p.From.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileConstToMemory implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileConstToMemory(
	inst asm.Instruction,
	constValue asm.ConstantValue,
	baseReg asm.Register,
	offset asm.ConstantValue,
) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileMemoryToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryToRegister(
	inst asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	destinationReg asm.Register,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	p.From.Offset = sourceOffsetConst
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(p)
}

// CompileMemoryToConst implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryToConst(
	inst asm.Instruction,
	baseReg asm.Register,
	offset, constValue asm.ConstantValue,
) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[baseReg]
	p.From.Offset = offset
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileJump implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_BRANCH
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileJumpToRegister implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[reg]
	a.AddInstruction(p)
}

// CompileJumpToMemory implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileJumpToMemory(
	jmpInstruction asm.Instruction,
	baseReg asm.Register,
	offset asm.ConstantValue,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[baseReg]
	p.To.Offset = offset
	a.AddInstruction(p)
}

// CompileRegisterToRegisterWithMode implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToRegisterWithMode(
	inst asm.Instruction,
	from, to asm.Register,
	mode asm_amd64.Mode,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = int64(mode)
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.RestArgs = append(p.RestArgs,
		obj.Addr{Reg: castAsGolangAsmRegister[from], Type: obj.TYPE_REG})
	a.AddInstruction(p)
}

// CompileReadInstructionAddress implements the same method as documented on asm_amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileReadInstructionAddress(
	destinationRegister asm.Register,
	beforeAcquisitionTargetInstruction asm.Instruction,
) {
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
	asm_amd64.REG_AX:  x86.REG_AX,
	asm_amd64.REG_CX:  x86.REG_CX,
	asm_amd64.REG_DX:  x86.REG_DX,
	asm_amd64.REG_BX:  x86.REG_BX,
	asm_amd64.REG_SP:  x86.REG_SP,
	asm_amd64.REG_BP:  x86.REG_BP,
	asm_amd64.REG_SI:  x86.REG_SI,
	asm_amd64.REG_DI:  x86.REG_DI,
	asm_amd64.REG_R8:  x86.REG_R8,
	asm_amd64.REG_R9:  x86.REG_R9,
	asm_amd64.REG_R10: x86.REG_R10,
	asm_amd64.REG_R11: x86.REG_R11,
	asm_amd64.REG_R12: x86.REG_R12,
	asm_amd64.REG_R13: x86.REG_R13,
	asm_amd64.REG_R14: x86.REG_R14,
	asm_amd64.REG_R15: x86.REG_R15,
	asm_amd64.REG_X0:  x86.REG_X0,
	asm_amd64.REG_X1:  x86.REG_X1,
	asm_amd64.REG_X2:  x86.REG_X2,
	asm_amd64.REG_X3:  x86.REG_X3,
	asm_amd64.REG_X4:  x86.REG_X4,
	asm_amd64.REG_X5:  x86.REG_X5,
	asm_amd64.REG_X6:  x86.REG_X6,
	asm_amd64.REG_X7:  x86.REG_X7,
	asm_amd64.REG_X8:  x86.REG_X8,
	asm_amd64.REG_X9:  x86.REG_X9,
	asm_amd64.REG_X10: x86.REG_X10,
	asm_amd64.REG_X11: x86.REG_X11,
	asm_amd64.REG_X12: x86.REG_X12,
	asm_amd64.REG_X13: x86.REG_X13,
	asm_amd64.REG_X14: x86.REG_X14,
	asm_amd64.REG_X15: x86.REG_X15,
}

// castAsGolangAsmRegister maps the instructions to golang-asm specific instruction values.
var castAsGolangAsmInstruction = [...]obj.As{
	asm_amd64.NOP:       obj.ANOP,
	asm_amd64.RET:       obj.ARET,
	asm_amd64.JMP:       obj.AJMP,
	asm_amd64.UD2:       x86.AUD2,
	asm_amd64.ADDL:      x86.AADDL,
	asm_amd64.ADDQ:      x86.AADDQ,
	asm_amd64.ADDSD:     x86.AADDSD,
	asm_amd64.ADDSS:     x86.AADDSS,
	asm_amd64.ANDL:      x86.AANDL,
	asm_amd64.ANDPD:     x86.AANDPD,
	asm_amd64.ANDPS:     x86.AANDPS,
	asm_amd64.ANDQ:      x86.AANDQ,
	asm_amd64.BSRL:      x86.ABSRL,
	asm_amd64.BSRQ:      x86.ABSRQ,
	asm_amd64.CDQ:       x86.ACDQ,
	asm_amd64.CMOVQCS:   x86.ACMOVQCS,
	asm_amd64.CMPL:      x86.ACMPL,
	asm_amd64.CMPQ:      x86.ACMPQ,
	asm_amd64.COMISD:    x86.ACOMISD,
	asm_amd64.COMISS:    x86.ACOMISS,
	asm_amd64.CQO:       x86.ACQO,
	asm_amd64.CVTSD2SS:  x86.ACVTSD2SS,
	asm_amd64.CVTSL2SD:  x86.ACVTSL2SD,
	asm_amd64.CVTSL2SS:  x86.ACVTSL2SS,
	asm_amd64.CVTSQ2SD:  x86.ACVTSQ2SD,
	asm_amd64.CVTSQ2SS:  x86.ACVTSQ2SS,
	asm_amd64.CVTSS2SD:  x86.ACVTSS2SD,
	asm_amd64.CVTTSD2SL: x86.ACVTTSD2SL,
	asm_amd64.CVTTSD2SQ: x86.ACVTTSD2SQ,
	asm_amd64.CVTTSS2SL: x86.ACVTTSS2SL,
	asm_amd64.CVTTSS2SQ: x86.ACVTTSS2SQ,
	asm_amd64.DECQ:      x86.ADECQ,
	asm_amd64.DIVL:      x86.ADIVL,
	asm_amd64.DIVQ:      x86.ADIVQ,
	asm_amd64.DIVSD:     x86.ADIVSD,
	asm_amd64.DIVSS:     x86.ADIVSS,
	asm_amd64.IDIVL:     x86.AIDIVL,
	asm_amd64.IDIVQ:     x86.AIDIVQ,
	asm_amd64.INCQ:      x86.AINCQ,
	asm_amd64.JCC:       x86.AJCC,
	asm_amd64.JCS:       x86.AJCS,
	asm_amd64.JEQ:       x86.AJEQ,
	asm_amd64.JGE:       x86.AJGE,
	asm_amd64.JGT:       x86.AJGT,
	asm_amd64.JHI:       x86.AJHI,
	asm_amd64.JLE:       x86.AJLE,
	asm_amd64.JLS:       x86.AJLS,
	asm_amd64.JLT:       x86.AJLT,
	asm_amd64.JMI:       x86.AJMI,
	asm_amd64.JNE:       x86.AJNE,
	asm_amd64.JPC:       x86.AJPC,
	asm_amd64.JPL:       x86.AJPL,
	asm_amd64.JPS:       x86.AJPS,
	asm_amd64.LEAQ:      x86.ALEAQ,
	asm_amd64.LZCNTL:    x86.ALZCNTL,
	asm_amd64.LZCNTQ:    x86.ALZCNTQ,
	asm_amd64.MAXSD:     x86.AMAXSD,
	asm_amd64.MAXSS:     x86.AMAXSS,
	asm_amd64.MINSD:     x86.AMINSD,
	asm_amd64.MINSS:     x86.AMINSS,
	asm_amd64.MOVB:      x86.AMOVB,
	asm_amd64.MOVBLSX:   x86.AMOVBLSX,
	asm_amd64.MOVBLZX:   x86.AMOVBLZX,
	asm_amd64.MOVBQSX:   x86.AMOVBQSX,
	asm_amd64.MOVBQZX:   x86.AMOVBQZX,
	asm_amd64.MOVL:      x86.AMOVL,
	asm_amd64.MOVLQSX:   x86.AMOVLQSX,
	asm_amd64.MOVLQZX:   x86.AMOVLQZX,
	asm_amd64.MOVQ:      x86.AMOVQ,
	asm_amd64.MOVW:      x86.AMOVW,
	asm_amd64.MOVWLSX:   x86.AMOVWLSX,
	asm_amd64.MOVWLZX:   x86.AMOVWLZX,
	asm_amd64.MOVWQSX:   x86.AMOVWQSX,
	asm_amd64.MOVWQZX:   x86.AMOVWQZX,
	asm_amd64.MULL:      x86.AMULL,
	asm_amd64.MULQ:      x86.AMULQ,
	asm_amd64.MULSD:     x86.AMULSD,
	asm_amd64.MULSS:     x86.AMULSS,
	asm_amd64.ORL:       x86.AORL,
	asm_amd64.ORPD:      x86.AORPD,
	asm_amd64.ORPS:      x86.AORPS,
	asm_amd64.ORQ:       x86.AORQ,
	asm_amd64.POPCNTL:   x86.APOPCNTL,
	asm_amd64.POPCNTQ:   x86.APOPCNTQ,
	asm_amd64.PSLLL:     x86.APSLLL,
	asm_amd64.PSLLQ:     x86.APSLLQ,
	asm_amd64.PSRLL:     x86.APSRLL,
	asm_amd64.PSRLQ:     x86.APSRLQ,
	asm_amd64.ROLL:      x86.AROLL,
	asm_amd64.ROLQ:      x86.AROLQ,
	asm_amd64.RORL:      x86.ARORL,
	asm_amd64.RORQ:      x86.ARORQ,
	asm_amd64.ROUNDSD:   x86.AROUNDSD,
	asm_amd64.ROUNDSS:   x86.AROUNDSS,
	asm_amd64.SARL:      x86.ASARL,
	asm_amd64.SARQ:      x86.ASARQ,
	asm_amd64.SETCC:     x86.ASETCC,
	asm_amd64.SETCS:     x86.ASETCS,
	asm_amd64.SETEQ:     x86.ASETEQ,
	asm_amd64.SETGE:     x86.ASETGE,
	asm_amd64.SETGT:     x86.ASETGT,
	asm_amd64.SETHI:     x86.ASETHI,
	asm_amd64.SETLE:     x86.ASETLE,
	asm_amd64.SETLS:     x86.ASETLS,
	asm_amd64.SETLT:     x86.ASETLT,
	asm_amd64.SETMI:     x86.ASETMI,
	asm_amd64.SETNE:     x86.ASETNE,
	asm_amd64.SETPC:     x86.ASETPC,
	asm_amd64.SETPL:     x86.ASETPL,
	asm_amd64.SETPS:     x86.ASETPS,
	asm_amd64.SHLL:      x86.ASHLL,
	asm_amd64.SHLQ:      x86.ASHLQ,
	asm_amd64.SHRL:      x86.ASHRL,
	asm_amd64.SHRQ:      x86.ASHRQ,
	asm_amd64.SQRTSD:    x86.ASQRTSD,
	asm_amd64.SQRTSS:    x86.ASQRTSS,
	asm_amd64.SUBL:      x86.ASUBL,
	asm_amd64.SUBQ:      x86.ASUBQ,
	asm_amd64.SUBSD:     x86.ASUBSD,
	asm_amd64.SUBSS:     x86.ASUBSS,
	asm_amd64.TESTL:     x86.ATESTL,
	asm_amd64.TESTQ:     x86.ATESTQ,
	asm_amd64.TZCNTL:    x86.ATZCNTL,
	asm_amd64.TZCNTQ:    x86.ATZCNTQ,
	asm_amd64.UCOMISD:   x86.AUCOMISD,
	asm_amd64.UCOMISS:   x86.AUCOMISS,
	asm_amd64.XORL:      x86.AXORL,
	asm_amd64.XORPD:     x86.AXORPD,
	asm_amd64.XORPS:     x86.AXORPS,
	asm_amd64.XORQ:      x86.AXORQ,
}
