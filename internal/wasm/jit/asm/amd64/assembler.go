package amd64

import (
	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

func NewAssembler() (Assembler, error) {
	return newGolangAsmAssembler() // TODO: replace with our homemade assembler #233
}

type Assembler interface {
	asm.AssemblerBase
	// CompileRegisterToRegisterWithMode adds an instruction where source and destination
	// are `from` and `to` registers and the instruction's "mode" is specified by `mode`.
	// For example, ROUND** instructions can be modified "mode" constant.
	// See https://www.felixcloutier.com/x86/roundss for ROUNDSS as an example.
	CompileRegisterToRegisterWithMode(instruction asm.Instruction, from, to asm.Register, mode int64)
	// CompileMemoryWithIndexToRegister adds an instruction where source operand is the memory address
	// specified as `srcBaseReg + srcOffsetConst + srcIndex*srcScale` and destination is the register `dstReg`.
	// Note: sourceScale must be one of 1, 2, 4, 8.
	CompileMemoryWithIndexToRegister(instruction asm.Instruction, srcBaseReg asm.Register, srcOffsetConst int64, srcIndex asm.Register, srcScale int16, dstReg asm.Register)
	// CompileRegisterToMemoryWithIndex adds an instruction where source operand is the register `srcReg`,
	// and the destination is the memory address specified as `dstBaseReg + dstOffsetConst + dstIndex*dstScale`
	// Note: dstScale must be one of 1, 2, 4, 8.
	CompileRegisterToMemoryWithIndex(instruction asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale int16)
	// CompileRegisterToConst adds an instruction where source operand is the register `srcRegister`,
	// and the destination is the const `value`.
	CompileRegisterToConst(instruction asm.Instruction, srcRegister asm.Register, value int64) asm.Node
	// CompileRegisterToNone adds an instruction where source operand is the register `register`,
	// and there's no destination operand.
	CompileRegisterToNone(instruction asm.Instruction, register asm.Register)
	// CompileRegisterToNone adds an instruction where destination operand is the register `register`,
	// and there's no source operand.
	CompileNoneToRegister(instruction asm.Instruction, register asm.Register)
	// CompileRegisterToNone adds an instruction where destination operand is the memory address specified
	// as `baseReg+offset`. and there's no source operand.
	CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset int64)
	// CompileConstToMemory adds an instruction where source operand is the constant `value` and
	// the destination is the memory address sppecified as `dstbaseReg+dstOffset`.
	CompileConstToMemory(instruction asm.Instruction, value int64, dstbaseReg asm.Register, dstOffset int64) asm.Node
	// CompileMemoryToConst adds an instruction where source operand is the memory address, and
	// the destination is the constant `value`.
	CompileMemoryToConst(instruction asm.Instruction, srcBaseReg asm.Register, srcOffset int64, value int64) asm.Node
}
