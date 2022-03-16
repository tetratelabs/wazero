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

var castAsGolangAsmInstruction = map[asm.Instruction]obj.As{
	JMP: obj.AJMP,
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

func (a *assemblerGoAsmImpl) CompileRegisterToRegister(inst asm.Instruction, from, to asm.Register) {
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
