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
	CompileStandAlone(asm.Instruction) asm.Node
	// TODO
	CompileRegisterToRegister(inst asm.Instruction, from, to asm.Register)
	// TODO
	CompileConstModeRegisterToRegister(inst asm.Instruction, from, to asm.Register, mode int64)
	// TODO
	CompileMemoryToRegister(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register)
	// TODO
	CompileMemoryWithIndexToRegister(inst asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, sourceIndex asm.Register, sourceScale int16, destinationReg asm.Register)
	// TODO
	CompileRegisterToMemoryWithIndex(inst asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale int16)
	// TODO
	CompileRegisterToMemory(inst asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64)
	// TODO
	CompileConstToRegister(inst asm.Instruction, constValue int64, destinationRegister asm.Register) asm.Node
	// TODO
	CompileRegisterToConst(inst asm.Instruction, srcRegister asm.Register, constValue int64) asm.Node
	// TODO
	CompileRegisterToNone(inst asm.Instruction, register asm.Register)
	// TODO
	CompileNoneToRegister(inst asm.Instruction, register asm.Register)
	// TODO
	CompileNoneToMemory(inst asm.Instruction, baseReg asm.Register, offset int64)
	// TODO
	CompileConstToMemory(inst asm.Instruction, constValue int64, baseReg asm.Register, offset int64) asm.Node
	// TODO
	CompileMemoryToConst(inst asm.Instruction, baseReg asm.Register, offset int64, constValue int64) asm.Node
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
