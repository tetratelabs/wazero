package asm_arm64

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/golang_asm"
)

// NewAssembler implements asm.NewAssembler and is used by default.
// This returns an implementation of Assembler interface via our homemade assembler implementation.
func NewAssembler(temporaryRegister asm.Register) (Assembler, error) {
	g, err := golang_asm.NewGolangAsmBaseAssembler("arm64")
	return &assemblerGoAsmImpl{GolangAsmBaseAssembler: g, temporaryRegister: temporaryRegister}, err
}

// Assembler is the interface for arm64 specific assembler.
type Assembler interface {
	asm.AssemblerBase
	// CompileMemoryWithRegisterOffsetToRegister adds an instruction where source operand is the memory address
	// specified as `srcBaseReg + srcOffsetReg` and dst is the register `dstReg`.
	CompileMemoryWithRegisterOffsetToRegister(instruction asm.Instruction, srcBaseReg, srcOffsetReg, dstReg asm.Register)
	// CompileRegisterToMemoryWithRegisterOffset adds an instruction where source operand is the register `srcReg`,
	// and the destination is the memory address specified as `dstBaseReg + dstOffsetReg`
	CompileRegisterToMemoryWithRegisterOffset(instruction asm.Instruction, srcReg, dstBaseReg, dstOffsetReg asm.Register)
	// CompileTwoRegistersToRegister adds an instruction where source operands consists of two registers `src1` and `src2`,
	// and the destination is the register `dst`.
	CompileTwoRegistersToRegister(instruction asm.Instruction, src1, src2, dst asm.Register)
	// CompileTwoRegisters adds an instruction where source operands consist of two registers `src1` and `src2`,
	// and destination operands consist of `dst1` and `dst2` registers.
	CompileTwoRegisters(instruction asm.Instruction, src1, src2, dst1, dst2 asm.Register)
	// CompileTwoRegistersToNone adds an instruction where source operands consist of two registers `src1` and `src2`,
	// and destination operand is unspecified.
	CompileTwoRegistersToNone(instruction asm.Instruction, src1, src2 asm.Register)
	// CompileRegisterAndConstSourceToNone adds an instruction where source operands consist of one register `src` and
	// constant `srcConst`, and destination operand is unspecified.
	CompileRegisterAndConstSourceToNone(instruction asm.Instruction, src asm.Register, srcConst asm.ConstantValue)
	// CompileLeftShiftedRegisterToRegister adds an instruction where source operand is the "left shifted register"
	// represented as `srcReg << shiftNum` and the destaintion is the register `dstReg`.
	CompileLeftShiftedRegisterToRegister(shiftedSourceReg asm.Register, shiftNum asm.ConstantValue, srcReg, dstReg asm.Register)
	// CompileSIMDByteToSIMDByte adds an instruction where source and destination operand is the SIMD register
	// specified as `srcReg.B8` and `dstReg.B8` where `.B8` part of register is called "arrangement".
	// See https://stackoverflow.com/questions/57294672/what-is-arrangement-specifier-16b-8b-in-arm-assembly-language-instructions
	CompileSIMDByteToSIMDByte(instruction asm.Instruction, srcReg, dstReg asm.Register)
	// CompileTwoSIMDByteToRegister adds an instruction where source operand is two SIMD registers specified as `srcReg1.B8`,
	// and `srcReg2.B8` and the destination is the register `dstReg`.
	CompileTwoSIMDByteToRegister(instruction asm.Instruction, srcReg1, srcReg2, dstReg asm.Register)
	// CompileSIMDByteToRegister adds an instruction where source operand is the SIMD register specified as `srcReg.B8`,
	// and the destination is the register `dstReg`.
	CompileSIMDByteToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register)
	// CompileConditionalRegisterSet adds an instruction to set 1 on dstReg if the condition satisfies,
	// otherwise set 0.
	CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, dstReg asm.Register)
}
