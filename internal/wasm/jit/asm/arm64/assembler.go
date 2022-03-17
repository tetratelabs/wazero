package arm64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

func NewAssembler(temporaryRegister asm.Register) (Assembler, error) {
	return newGolangAsmAssembler(temporaryRegister) // TODO: replace our homemade assembler #233
}

type Assembler interface {
	asm.AssemblerBase
	// TODO
	CompileConstToRegisterInstruction(instruction asm.Instruction, constValue int64, destinationReg asm.Register) (inst asm.Node)
	// TODO
	CompileMemoryToRegisterInstruction(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	// TODO
	CompileMemoryWithRegisterOffsetToRegisterInstruction(instruction asm.Instruction, sourceBaseReg, sourceOffsetReg, destinationReg asm.Register)
	// TODO
	CompileRegisterToMemoryInstruction(instruction asm.Instruction, sourceReg asm.Register, destinationBaseReg asm.Register, destinationOffsetConst int64)
	// TODO
	CompileRegisterToMemoryWithRegisterOffsetInstruction(instruction asm.Instruction, sourceRegister, destinationBaseRegister, destinationOffsetReg asm.Register)
	// TODO
	CompileRegisterToRegisterInstruction(instruction asm.Instruction, from, to asm.Register)
	// TODO
	CompileTwoRegistersToRegisterInstruction(instruction asm.Instruction, src1, src2, destination asm.Register)
	// TODO
	CompileTwoRegistersInstruction(instruction asm.Instruction, src1, src2, dst1, dst2 asm.Register)
	// TODO
	CompileTwoRegistersToNoneInstruction(instruction asm.Instruction, src1, src2 asm.Register)
	// TODO
	CompileRegisterAndConstSourceToNoneInstruction(instruction asm.Instruction, src asm.Register, srcConst int64)
	// TODO
	CompileBranchInstruction(instruction asm.Instruction) (br asm.Node)
	// TODO
	CompileUnconditionalBranchToAddressOnMemory(addressReg asm.Register)
	// TODO
	CompileStandAloneInstruction(instruction asm.Instruction) asm.Node
	// TODO
	CompileAddInstructionWithLeftShiftedRegister(shiftedSourceReg asm.Register, shiftNum int64, srcReg, desReg asm.Register)
	// TODO
	CompileReturn(returnAddressReg asm.Register)

	CompileSIMDToSIMDWithByteArrangement(instruction asm.Instruction, srcReg, dstReg asm.Register)
	CompileTwoSIMDToSIMDWithByteArrangement(instruction asm.Instruction, srcReg1, srcReg2, dstReg asm.Register)
	CompileSIMDWithByteArrangementToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register)

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
