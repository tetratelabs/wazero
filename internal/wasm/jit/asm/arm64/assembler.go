package arm64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

func NewAssembler(temporaryRegister asm.Register) (Assembler, error) {
	return newGolangAsmAssembler(temporaryRegister) // TODO: replace our homemade assembler #233
}

type Assembler interface {
	asm.AssemblerBase
	CompileConstToRegisterInstruction(instruction asm.Instruction, constValue int64, destinationReg asm.Register) (inst asm.Node)
	CompileMemoryToRegisterInstruction(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	CompileMemoryWithRegisterOffsetToRegisterInstruction(instruction asm.Instruction, sourceBaseReg, sourceOffsetReg, destinationReg asm.Register)
	CompileRegisterToMemoryInstruction(instruction asm.Instruction, sourceReg asm.Register, destinationBaseReg asm.Register, destinationOffsetConst int64)
	CompileRegisterToMemoryWithRegisterOffsetInstruction(instruction asm.Instruction, sourceRegister, destinationBaseRegister, destinationOffsetReg asm.Register)
	CompileRegisterToRegisterInstruction(instruction asm.Instruction, from, to asm.Register)
	CompileTwoRegistersToRegisterInstruction(instruction asm.Instruction, src1, src2, destination asm.Register)
	CompileTwoRegistersInstruction(instruction asm.Instruction, src1, src2, dst1, dst2 asm.Register)
	CompileTwoRegistersToNoneInstruction(instruction asm.Instruction, src1, src2 asm.Register)
	CompileRegisterAndConstSourceToNoneInstruction(instruction asm.Instruction, src asm.Register, srcConst int64)
	CompileBranchInstruction(instruction asm.Instruction) (br asm.Node)
	CompileUnconditionalBranchToAddressOnMemory(addressReg asm.Register)
	CompileStandAloneInstruction(instruction asm.Instruction) asm.Node
	CompileAddInstructionWithLeftShiftedRegister(shiftedSourceReg asm.Register, shiftNum int64, srcReg, destinationReg asm.Register)
	CompileReturn(returnAddressReg asm.Register)

	// CompileReadInstructionAddress adds an ADR instruction to set the absolute address of "target instruction"
	// into destinationRegister. "target instruction" is specified by beforeTargetInst argument and
	// the target is determined by "the instruction right after beforeTargetInst type".
	//
	// For example, if beforeTargetInst == RET and we have the instruction sequence like
	// ADR -> X -> Y -> ... -> RET -> MOV, then the ADR instruction emitted by this function set the absolute
	// address of MOV instruction into the destination register.
	CompileReadInstructionAddress(beforeTargetInst asm.Instruction, destinationRegister asm.Register)

	BuildJumpTable(table []byte, initialInstructions []asm.Node)

	// CSET
	CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, destinationReg asm.Register)
}
