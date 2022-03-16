package amd64

import (
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
	goasm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

type assemblerGoAsmImpl struct {
	b                          *goasm.Builder
	setBranchTargetOnNextNodes []asm.Node
	// onGenerateCallbacks holds the callbacks which are called AFTER generating native code.
	onGenerateCallbacks []func(code []byte) error
}

var _ Assembler = &assemblerGoAsmImpl{}

func newGolangAsmAssembler() (*assemblerGoAsmImpl, error) {
	b, err := goasm.NewBuilder("arm64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	return &assemblerGoAsmImpl{b: b}, nil
}

func (a *assemblerGoAsmImpl) newProg() (prog *obj.Prog) {
	prog = a.b.NewProg()
	return
}

func (a *assemblerGoAsmImpl) addInstruction(next *obj.Prog) {
	a.b.AddInstruction(next)
	for _, node := range a.setBranchTargetOnNextNodes {
		prog := node.(*obj.Prog)
		prog.To.SetTarget(next)
	}
	a.setBranchTargetOnNextNodes = nil
}

func (a *assemblerGoAsmImpl) Assemble() []byte {
	code := a.b.Assemble()
	for _, cb := range a.onGenerateCallbacks {
		cb(code)
	}
	return code
}

func (a *assemblerGoAsmImpl) SetBranchTargetOnNext(nodes ...asm.Node) {
	a.setBranchTargetOnNextNodes = append(a.setBranchTargetOnNextNodes, nodes...)
}

func (a *assemblerGoAsmImpl) CompileStandAloneInstruction(inst asm.Instruction) asm.Node {
	prog := a.newProg()
	prog.As = castAsGolangAsmInstruction[inst]
	a.addInstruction(prog)
	return prog
}

func (a *assemblerGoAsmImpl) CompileRegisterToRegisterInstruction(inst asm.Instruction, from, to asm.Register) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = to
	p.From.Type = obj.TYPE_REG
	p.From.Reg = from
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileMemoryWithIndexToRegisterInstruction(inst asm.Register,
	sourceBaseReg asm.Register, sourceOffsetConst int64, sourceIndex asm.Register, sourceScale asm.Register, destinationReg asm.Register) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = destinationReg
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = sourceBaseReg
	p.From.Offset = sourceOffsetConst
	p.From.Index = sourceIndex
	p.From.Scale = sourceScale
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithIndexInstruction(inst asm.Register, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale asm.Register) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = srcReg
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = dstBaseReg
	p.To.Offset = dstOffsetConst
	p.To.Index = dstIndex
	p.To.Scale = dstScale
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileRegisterToMemoryInstruction(inst asm.Register, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = destinationBaseRegister
	p.To.Offset = destinationOffsetConst
	p.From.Type = obj.TYPE_REG
	p.From.Reg = sourceRegister
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileConstToRegisterInstruction(inst asm.Register, constValue int64, destinationRegister asm.Register) asm.Node {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_REG
	p.To.Reg = destinationRegister
	a.addInstruction(p)
	return p
}

func (a *assemblerGoAsmImpl) CompileRegisterToConstInstruction(inst asm.Register, srcRegister asm.Register, constValue int64) asm.Node {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_REG
	p.From.Reg = srcRegister
	a.addInstruction(p)
	return p
}

func (a *assemblerGoAsmImpl) CompileRegisterToNoneInstruction(inst asm.Register, register asm.Register) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_REG
	p.From.Reg = register
	p.To.Type = obj.TYPE_NONE
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileNoneToRegisterInstruction(inst asm.Register, register asm.Register) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_REG
	p.To.Reg = register
	p.From.Type = obj.TYPE_NONE
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileNoneToMemoryInstruction(inst asm.Register, baseReg asm.Register, offset int64) {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = baseReg
	p.To.Offset = offset
	p.From.Type = obj.TYPE_NONE
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileConstToMemoryInstruction(inst asm.Register, constValue int64, baseReg asm.Register, offset int64) asm.Node {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_CONST
	p.From.Offset = constValue
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = baseReg
	p.To.Offset = offset
	a.addInstruction(p)
	return p
}

func (c *assemblerGoAsmImpl) CompileMemoryToRegisterInstruction(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register) {
	p := c.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = sourceBaseReg
	p.From.Offset = sourceOffsetConst
	p.To.Type = obj.TYPE_REG
	p.To.Reg = destinationReg
	c.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileMemoryToConstInstruction(inst asm.Register, baseReg asm.Register, offset int64, constValue int64) asm.Node {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_CONST
	p.To.Offset = constValue
	p.From.Type = obj.TYPE_MEM
	p.From.Reg = baseReg
	p.From.Offset = offset
	a.addInstruction(p)
	return p
}

func (a *assemblerGoAsmImpl) CompileUnconditionalJump() asm.Node {
	return a.CompileJump(JMP)
}

func (a *assemblerGoAsmImpl) CompileJump(inst asm.Instruction) asm.Node {
	p := a.newProg()
	p.As = castAsGolangAsmInstruction[inst]
	p.To.Type = obj.TYPE_BRANCH
	a.addInstruction(p)
	return p
}

func (a *assemblerGoAsmImpl) CompileJumpToRegister(reg asm.Register) {
	p := a.newProg()
	p.As = obj.AJMP
	p.To.Type = obj.TYPE_REG
	p.To.Reg = reg
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileJumpToMemory(baseReg asm.Register, offset int64) {
	p := a.newProg()
	p.As = obj.AJMP
	p.To.Type = obj.TYPE_MEM
	p.To.Reg = baseReg
	p.To.Offset = offset
	a.addInstruction(p)
}

func (a *assemblerGoAsmImpl) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	// Emit the instruction in the form of "LEA destination [RIP + offset]".
	readInstructionAddress := a.newProg()
	readInstructionAddress.As = x86.ALEAQ
	readInstructionAddress.To.Reg = destinationRegister
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
	a.addInstruction(readInstructionAddress)

	a.onGenerateCallbacks = append(a.onGenerateCallbacks, func(code []byte) error {
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

var castAsGolangAsmInstruction = map[asm.Instruction]obj.As{
	NOP:       obj.ANOP,
	RET:       obj.ARET,
	JMP:       obj.AJMP,
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
