package amd64_debug

import (
	"encoding/binary"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/integration_test/asm/golang_asm"
)

// assemblerGoAsmImpl implements amd64.Assembler for golang-asm library.
type assemblerGoAsmImpl struct {
	*golang_asm.GolangAsmBaseAssembler
}

func newGolangAsmAssembler() (*assemblerGoAsmImpl, error) {
	g, err := golang_asm.NewGolangAsmBaseAssembler("amd64")
	return &assemblerGoAsmImpl{g}, err
}

// CompileStandAlone implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileStandAlone(inst asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileRegisterToRegister implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToRegister(inst asm.Instruction, from, to asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[from]
	a.AddInstruction(p)
}

// CompileMemoryWithIndexToRegister implements the same method as documented on amd64.Assembler.
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

// CompileMemoryWithIndexAndArgToRegister implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryWithIndexAndArgToRegister(
	inst asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	sourceIndexReg asm.Register,
	sourceScale int16,
	destinationReg asm.Register,
	arg byte,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationReg]
	p.RestArgs = append(p.RestArgs,
		obj.Addr{
			Reg:    castAsGolangAsmRegister[sourceBaseReg],
			Offset: sourceOffsetConst,
			Index:  castAsGolangAsmRegister[sourceIndexReg],
			Scale:  sourceScale,
			Type:   obj.TYPE_MEM,
		})

	p.From.Type = obj.TYPE_CONST
	p.From.Offset = int64(arg)
	a.AddInstruction(p)
}

// CompileRegisterToMemoryWithIndex implements the same method as documented on amd64.Assembler.
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

// CompileRegisterToMemoryWithIndexAndArg implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithIndexAndArg(
	inst asm.Instruction,
	srcReg, dstBaseReg asm.Register,
	dstOffsetConst asm.ConstantValue,
	dstIndexReg asm.Register,
	dstScale int16,
	arg byte,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = int64(arg)
	p.RestArgs = append(p.RestArgs,
		obj.Addr{Reg: castAsGolangAsmRegister[srcReg], Type: obj.TYPE_REG})

	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[dstBaseReg]
	p.To.Offset = dstOffsetConst
	p.To.Index = castAsGolangAsmRegister[dstIndexReg]
	p.To.Scale = dstScale
	a.AddInstruction(p)
}

// CompileRegisterToMemory implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToMemory(
	inst asm.Instruction,
	sourceRegister, destinationBaseRegister asm.Register,
	destinationOffsetConst asm.ConstantValue,
) {
	if inst == amd64.MOVDQU {
		panic("unsupported by golang-asm")
	}
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = castAsGolangAsmRegister[destinationBaseRegister]
	p.To.Offset = destinationOffsetConst
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[sourceRegister]
	a.AddInstruction(p)
}

// CompileConstToRegister implements the same method as documented on amd64.Assembler.
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

// CompileRegisterToConst implements the same method as documented on amd64.Assembler.
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

// CompileRegisterToNone implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToNone(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = castAsGolangAsmRegister[register]
	p.To.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToRegister implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileNoneToRegister(inst asm.Instruction, register asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[register]
	p.From.Type = obj.TYPE_NONE
	a.AddInstruction(p)
}

// CompileNoneToMemory implements the same method as documented on amd64.Assembler.
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

// CompileConstToMemory implements the same method as documented on amd64.Assembler.
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

// CompileMemoryToRegister implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryToRegister(
	inst asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	destinationReg asm.Register,
) {
	if inst == amd64.MOVDQU {
		panic("unsupported by golang-asm")
	}
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	p.From.Offset = sourceOffsetConst
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(p)
}

// CompileMemoryToConst implements the same method as documented on amd64.Assembler.
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

// CompileJump implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_BRANCH
	a.AddInstruction(p)
	return golang_asm.NewGolangAsmNode(p)
}

// CompileJumpToRegister implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[jmpInstruction]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[reg]
	a.AddInstruction(p)
}

// CompileJumpToMemory implements the same method as documented on amd64.Assembler.
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

// CompileRegisterToRegisterWithArg implements the same method as documented on amd64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToRegisterWithArg(
	inst asm.Instruction,
	from, to asm.Register,
	arg byte,
) {
	p := a.NewProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = castAsGolangAsmRegister[to]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = int64(arg)
	p.RestArgs = append(p.RestArgs,
		obj.Addr{Reg: castAsGolangAsmRegister[from], Type: obj.TYPE_REG})
	a.AddInstruction(p)
}

// CompileReadInstructionAddress implements the same method as documented on amd64.Assembler.
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
	// We intentionally use x86.RegBP here so that the resulting instruction sequence becomes
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

// CompileStaticConstToRegister implements Assembler.CompileStaticConstToRegister.
func (a *assemblerGoAsmImpl) CompileStaticConstToRegister(asm.Instruction, *asm.StaticConst, asm.Register) (err error) {
	panic("CompileStaticConstToRegister cannot be supported by golangasm")
}

// CompileRegisterToStaticConst implements Assembler.CompileRegisterToStaticConst.
func (a *assemblerGoAsmImpl) CompileRegisterToStaticConst(asm.Instruction, asm.Register, *asm.StaticConst) (err error) {
	panic("CompileRegisterToStaticConst cannot be supported by golang-asm")
}

// castAsGolangAsmRegister maps the registers to golang-asm specific register values.
var castAsGolangAsmRegister = [...]int16{
	amd64.RegAX:  x86.REG_AX,
	amd64.RegCX:  x86.REG_CX,
	amd64.RegDX:  x86.REG_DX,
	amd64.RegBX:  x86.REG_BX,
	amd64.RegSP:  x86.REG_SP,
	amd64.RegBP:  x86.REG_BP,
	amd64.RegSI:  x86.REG_SI,
	amd64.RegDI:  x86.REG_DI,
	amd64.RegR8:  x86.REG_R8,
	amd64.RegR9:  x86.REG_R9,
	amd64.RegR10: x86.REG_R10,
	amd64.RegR11: x86.REG_R11,
	amd64.RegR12: x86.REG_R12,
	amd64.RegR13: x86.REG_R13,
	amd64.RegR14: x86.REG_R14,
	amd64.RegR15: x86.REG_R15,
	amd64.RegX0:  x86.REG_X0,
	amd64.RegX1:  x86.REG_X1,
	amd64.RegX2:  x86.REG_X2,
	amd64.RegX3:  x86.REG_X3,
	amd64.RegX4:  x86.REG_X4,
	amd64.RegX5:  x86.REG_X5,
	amd64.RegX6:  x86.REG_X6,
	amd64.RegX7:  x86.REG_X7,
	amd64.RegX8:  x86.REG_X8,
	amd64.RegX9:  x86.REG_X9,
	amd64.RegX10: x86.REG_X10,
	amd64.RegX11: x86.REG_X11,
	amd64.RegX12: x86.REG_X12,
	amd64.RegX13: x86.REG_X13,
	amd64.RegX14: x86.REG_X14,
	amd64.RegX15: x86.REG_X15,
}

// castAsGolangAsmRegister maps the instructions to golang-asm specific instruction values.
var castAsGolangAsmInstruction = [...]obj.As{
	amd64.NOP:       obj.ANOP,
	amd64.RET:       obj.ARET,
	amd64.JMP:       obj.AJMP,
	amd64.UD2:       x86.AUD2,
	amd64.ADDL:      x86.AADDL,
	amd64.ADDQ:      x86.AADDQ,
	amd64.ADDSD:     x86.AADDSD,
	amd64.ADDSS:     x86.AADDSS,
	amd64.ANDL:      x86.AANDL,
	amd64.ANDPD:     x86.AANDPD,
	amd64.ANDPS:     x86.AANDPS,
	amd64.ANDQ:      x86.AANDQ,
	amd64.BSRL:      x86.ABSRL,
	amd64.BSRQ:      x86.ABSRQ,
	amd64.CDQ:       x86.ACDQ,
	amd64.CMOVQCS:   x86.ACMOVQCS,
	amd64.CMPL:      x86.ACMPL,
	amd64.CMPQ:      x86.ACMPQ,
	amd64.COMISD:    x86.ACOMISD,
	amd64.COMISS:    x86.ACOMISS,
	amd64.CQO:       x86.ACQO,
	amd64.CVTSD2SS:  x86.ACVTSD2SS,
	amd64.CVTSL2SD:  x86.ACVTSL2SD,
	amd64.CVTSL2SS:  x86.ACVTSL2SS,
	amd64.CVTSQ2SD:  x86.ACVTSQ2SD,
	amd64.CVTSQ2SS:  x86.ACVTSQ2SS,
	amd64.CVTSS2SD:  x86.ACVTSS2SD,
	amd64.CVTTSD2SL: x86.ACVTTSD2SL,
	amd64.CVTTSD2SQ: x86.ACVTTSD2SQ,
	amd64.CVTTSS2SL: x86.ACVTTSS2SL,
	amd64.CVTTSS2SQ: x86.ACVTTSS2SQ,
	amd64.DECQ:      x86.ADECQ,
	amd64.DIVL:      x86.ADIVL,
	amd64.DIVQ:      x86.ADIVQ,
	amd64.DIVSD:     x86.ADIVSD,
	amd64.DIVSS:     x86.ADIVSS,
	amd64.IDIVL:     x86.AIDIVL,
	amd64.IDIVQ:     x86.AIDIVQ,
	amd64.INCQ:      x86.AINCQ,
	amd64.JCC:       x86.AJCC,
	amd64.JCS:       x86.AJCS,
	amd64.JEQ:       x86.AJEQ,
	amd64.JGE:       x86.AJGE,
	amd64.JGT:       x86.AJGT,
	amd64.JHI:       x86.AJHI,
	amd64.JLE:       x86.AJLE,
	amd64.JLS:       x86.AJLS,
	amd64.JLT:       x86.AJLT,
	amd64.JMI:       x86.AJMI,
	amd64.JNE:       x86.AJNE,
	amd64.JPC:       x86.AJPC,
	amd64.JPL:       x86.AJPL,
	amd64.JPS:       x86.AJPS,
	amd64.LEAQ:      x86.ALEAQ,
	amd64.LZCNTL:    x86.ALZCNTL,
	amd64.LZCNTQ:    x86.ALZCNTQ,
	amd64.NEGQ:      x86.ANEGQ,
	amd64.MAXSD:     x86.AMAXSD,
	amd64.MAXSS:     x86.AMAXSS,
	amd64.MINSD:     x86.AMINSD,
	amd64.MINSS:     x86.AMINSS,
	amd64.MOVB:      x86.AMOVB,
	amd64.MOVBLSX:   x86.AMOVBLSX,
	amd64.MOVBLZX:   x86.AMOVBLZX,
	amd64.MOVBQSX:   x86.AMOVBQSX,
	amd64.MOVBQZX:   x86.AMOVBQZX,
	amd64.MOVL:      x86.AMOVL,
	amd64.MOVLQSX:   x86.AMOVLQSX,
	amd64.MOVLQZX:   x86.AMOVLQZX,
	amd64.MOVQ:      x86.AMOVQ,
	amd64.MOVW:      x86.AMOVW,
	amd64.MOVWLSX:   x86.AMOVWLSX,
	amd64.MOVWLZX:   x86.AMOVWLZX,
	amd64.MOVWQSX:   x86.AMOVWQSX,
	amd64.MOVWQZX:   x86.AMOVWQZX,
	amd64.MULL:      x86.AMULL,
	amd64.MULQ:      x86.AMULQ,
	amd64.MULSD:     x86.AMULSD,
	amd64.MULSS:     x86.AMULSS,
	amd64.ORL:       x86.AORL,
	amd64.ORPD:      x86.AORPD,
	amd64.ORPS:      x86.AORPS,
	amd64.ORQ:       x86.AORQ,
	amd64.POPCNTL:   x86.APOPCNTL,
	amd64.POPCNTQ:   x86.APOPCNTQ,
	amd64.PSLLD:     x86.APSLLL,
	amd64.PSLLQ:     x86.APSLLQ,
	amd64.PSRLD:     x86.APSRLL,
	amd64.PSRLQ:     x86.APSRLQ,
	amd64.ROLL:      x86.AROLL,
	amd64.ROLQ:      x86.AROLQ,
	amd64.RORL:      x86.ARORL,
	amd64.RORQ:      x86.ARORQ,
	amd64.ROUNDSD:   x86.AROUNDSD,
	amd64.ROUNDSS:   x86.AROUNDSS,
	amd64.SARL:      x86.ASARL,
	amd64.SARQ:      x86.ASARQ,
	amd64.SETCC:     x86.ASETCC,
	amd64.SETCS:     x86.ASETCS,
	amd64.SETEQ:     x86.ASETEQ,
	amd64.SETGE:     x86.ASETGE,
	amd64.SETGT:     x86.ASETGT,
	amd64.SETHI:     x86.ASETHI,
	amd64.SETLE:     x86.ASETLE,
	amd64.SETLS:     x86.ASETLS,
	amd64.SETLT:     x86.ASETLT,
	amd64.SETMI:     x86.ASETMI,
	amd64.SETNE:     x86.ASETNE,
	amd64.SETPC:     x86.ASETPC,
	amd64.SETPL:     x86.ASETPL,
	amd64.SETPS:     x86.ASETPS,
	amd64.SHLL:      x86.ASHLL,
	amd64.SHLQ:      x86.ASHLQ,
	amd64.SHRL:      x86.ASHRL,
	amd64.SHRQ:      x86.ASHRQ,
	amd64.SQRTSD:    x86.ASQRTSD,
	amd64.SQRTSS:    x86.ASQRTSS,
	amd64.SUBL:      x86.ASUBL,
	amd64.SUBQ:      x86.ASUBQ,
	amd64.SUBSD:     x86.ASUBSD,
	amd64.SUBSS:     x86.ASUBSS,
	amd64.TESTL:     x86.ATESTL,
	amd64.TESTQ:     x86.ATESTQ,
	amd64.TZCNTL:    x86.ATZCNTL,
	amd64.TZCNTQ:    x86.ATZCNTQ,
	amd64.UCOMISD:   x86.AUCOMISD,
	amd64.UCOMISS:   x86.AUCOMISS,
	amd64.XORL:      x86.AXORL,
	amd64.XORPD:     x86.AXORPD,
	amd64.XORPS:     x86.AXORPS,
	amd64.XORQ:      x86.AXORQ,
	amd64.PINSRB:    x86.APINSRB,
	amd64.PINSRW:    x86.APINSRW,
	amd64.PINSRD:    x86.APINSRD,
	amd64.PINSRQ:    x86.APINSRQ,
	amd64.PADDB:     x86.APADDB,
	amd64.PADDW:     x86.APADDW,
	amd64.PADDD:     x86.APADDL,
	amd64.PADDQ:     x86.APADDQ,
	amd64.ADDPS:     x86.AADDPS,
	amd64.ADDPD:     x86.AADDPD,
	amd64.PSUBB:     x86.APSUBB,
	amd64.PSUBW:     x86.APSUBW,
	amd64.PSUBD:     x86.APSUBL,
	amd64.PSUBQ:     x86.APSUBQ,
	amd64.SUBPS:     x86.ASUBPS,
	amd64.SUBPD:     x86.ASUBPD,
	amd64.PMOVSXBW:  x86.APMOVSXBW,
	amd64.PMOVSXWD:  x86.APMOVSXWD,
	amd64.PMOVSXDQ:  x86.APMOVSXDQ,
	amd64.PMOVZXBW:  x86.APMOVZXBW,
	amd64.PMOVZXWD:  x86.APMOVZXWD,
	amd64.PMOVZXDQ:  x86.APMOVZXDQ,
	amd64.PSHUFB:    x86.APSHUFB,
	amd64.PSHUFD:    x86.APSHUFD,
	amd64.PXOR:      x86.APXOR,
	amd64.PEXTRB:    x86.APEXTRB,
	amd64.PEXTRW:    x86.APEXTRW,
	amd64.PEXTRD:    x86.APEXTRD,
	amd64.PEXTRQ:    x86.APEXTRQ,
	amd64.MOVLHPS:   x86.AMOVLHPS,
	amd64.INSERTPS:  x86.AINSERTPS,
	amd64.PTEST:     x86.APTEST,
	amd64.PCMPEQB:   x86.APCMPEQB,
	amd64.PCMPEQW:   x86.APCMPEQW,
	amd64.PCMPEQD:   x86.APCMPEQL,
	amd64.PCMPEQQ:   x86.APCMPEQQ,
	amd64.PADDUSB:   x86.APADDUSB,
	amd64.MOVSD:     x86.AMOVSD,
}
