package amd64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

func NewAssembler() (Assembler, error) {
	return newGolangAsmAssembler() // TODO: replace our homemade assembler #233
}

type Assembler interface {
	asm.AssemblerBase

	// TODO
	CompileStandAloneInstruction(asm.Instruction) asm.Node
	// TODO
	CompileRegisterToRegisterInstruction(inst asm.Instruction, from, to asm.Register)
	// TODO
	CompileConstModeRegisterToRegisterInstruction(inst asm.Instruction, from, to asm.Register, mode int64)
	// TODO
	CompileMemoryToRegisterInstruction(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	// TODO
	CompileMemoryWithIndexToRegisterInstruction(inst asm.Register, sourceBaseReg asm.Register, sourceOffsetConst int64, sourceIndex asm.Register, sourceScale asm.Register, destinationReg asm.Register)
	// TODO
	CompileRegisterToMemoryWithIndexInstruction(inst asm.Register, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale asm.Register)
	// TODO
	CompileRegisterToMemoryInstruction(inst asm.Register, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64)
	// TODO
	CompileConstToRegisterInstruction(inst asm.Register, constValue int64, destinationRegister asm.Register) asm.Node
	// TODO
	CompileRegisterToConstInstruction(inst asm.Register, srcRegister asm.Register, constValue int64) asm.Node
	// TODO
	CompileRegisterToNoneInstruction(inst asm.Register, register asm.Register)
	// TODO
	CompileNoneToRegisterInstruction(inst asm.Register, register asm.Register)
	// TODO
	CompileNoneToMemoryInstruction(inst asm.Register, baseReg asm.Register, offset int64)
	// TODO
	CompileConstToMemoryInstruction(inst asm.Register, constValue int64, baseReg asm.Register, offset int64) asm.Node
	// TODO
	CompileMemoryToConstInstruction(inst asm.Register, baseReg asm.Register, offset int64, constValue int64) asm.Node
	// TODO
	CompileUnconditionalJump() asm.Node
	// TODO
	CompileJump(jmpInst asm.Instruction) asm.Node
	// TODO
	CompileJumpToRegister(reg asm.Register)
	// TODO
	CompileJumpToMemory(baseReg asm.Register, offset int64)
	// TODO
	CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction)
}
