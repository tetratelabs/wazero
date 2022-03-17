package arm64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

func NewAssembler(temporaryRegister asm.Register) (Assembler, error) {
	return newGolangAsmAssembler(temporaryRegister) // TODO: replace with our homemade assembler #233
}

// Assembler is the interface for arm64 specific assembler.
type Assembler interface {
	asm.AssemblerBase

	// TODO
	CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	// TODO
	CompileMemoryWithRegisterOffsetToRegister(instruction asm.Instruction, sourceBaseReg, sourceOffsetReg, destinationReg asm.Register)
	// TODO
	CompileRegisterToMemory(instruction asm.Instruction, sourceReg asm.Register, destinationBaseReg asm.Register, destinationOffsetConst int64)
	// TODO
	CompileRegisterToMemoryWithRegisterOffset(instruction asm.Instruction, sourceRegister, destinationBaseRegister, destinationOffsetReg asm.Register)
	// TODO
	CompileTwoRegistersToRegister(instruction asm.Instruction, src1, src2, destination asm.Register)
	// TODO
	CompileTwoRegisters(instruction asm.Instruction, src1, src2, dst1, dst2 asm.Register)
	// TODO
	CompileTwoRegistersToNone(instruction asm.Instruction, src1, src2 asm.Register)
	// TODO
	CompileRegisterAndConstSourceToNone(instruction asm.Instruction, src asm.Register, srcConst int64)
	// TODO
	CompileAddInstructionWithLeftShiftedRegister(shiftedSourceReg asm.Register, shiftNum int64, srcReg, desReg asm.Register)
	// TODO
	CompileSIMDToSIMDWithByteArrangement(instruction asm.Instruction, srcReg, dstReg asm.Register)
	// TODO
	CompileTwoSIMDToSIMDWithByteArrangement(instruction asm.Instruction, srcReg1, srcReg2, dstReg asm.Register)
	// TODO
	CompileSIMDWithByteArrangementToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register)
	// CompileReadInstructionAddress adds an ADR instruction to set the absolute address of "target instruction"
	// into destinationRegister. "target instruction" is specified by beforeTargetInst argument and
	// the target is determined by "the instruction right after beforeTargetInst type".
	//
	// For example, if beforeTargetInst == RET and we have the instruction sequence like
	// ADR -> X -> Y -> ... -> RET -> MOV, then the ADR instruction emitted by this function set the absolute
	// address of MOV instruction into the destination register.
	CompileReadInstructionAddress(beforeTargetInst asm.Instruction, destinationRegister asm.Register)
	// CompileConditionalRegisterSet adds an instruction to set 1 on destinationReg if the condition satisfies,
	// otherwise set 0.
	CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, destinationReg asm.Register)
}
