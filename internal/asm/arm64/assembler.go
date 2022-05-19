package arm64

import (
	"github.com/tetratelabs/wazero/internal/asm"
)

// Assembler is the interface for arm64 specific assembler.
type Assembler interface {
	asm.AssemblerBase

	// CompileJumpToMemory adds jump-type instruction whose destination is stored in the memory address specified by
	// `baseReg`, and returns the corresponding Node in the assembled linked list.
	//
	// Note: this has exactly the same implementation as the same method in asm.AssemblerBase in the homemade assembler.
	// TODO: this will be removed after golang-asm removal.
	CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register)

	// CompileMemoryWithRegisterOffsetToRegister adds an instruction where source operand is the memory address
	// specified as `srcBaseReg + srcOffsetReg` and dst is the register `dstReg`.
	CompileMemoryWithRegisterOffsetToRegister(instruction asm.Instruction, srcBaseReg, srcOffsetReg, dstReg asm.Register)

	// CompileRegisterToMemoryWithRegisterOffset adds an instruction where source operand is the register `srcReg`,
	// and the destination is the memory address specified as `dstBaseReg + dstOffsetReg`
	CompileRegisterToMemoryWithRegisterOffset(instruction asm.Instruction, srcReg, dstBaseReg, dstOffsetReg asm.Register)

	// CompileTwoRegistersToRegister adds an instruction where source operands consists of two registers `src1` and `src2`,
	// and the destination is the register `dst`.
	CompileTwoRegistersToRegister(instruction asm.Instruction, src1, src2, dst asm.Register)

	// CompileThreeRegistersToRegister adds an instruction where source operands consist of three registers
	// `src1`, `src2` and `src3`, and destination operands consist of `dst` register.
	CompileThreeRegistersToRegister(instruction asm.Instruction, src1, src2, src3, dst asm.Register)

	// CompileTwoRegistersToNone adds an instruction where source operands consist of two registers `src1` and `src2`,
	// and destination operand is unspecified.
	CompileTwoRegistersToNone(instruction asm.Instruction, src1, src2 asm.Register)

	// CompileRegisterAndConstToNone adds an instruction where source operands consist of one register `src` and
	// constant `srcConst`, and destination operand is unspecified.
	CompileRegisterAndConstToNone(instruction asm.Instruction, src asm.Register, srcConst asm.ConstantValue)

	// CompileLeftShiftedRegisterToRegister adds an instruction where source operand is the "left shifted register"
	// represented as `srcReg << shiftNum` and the destination is the register `dstReg`.
	CompileLeftShiftedRegisterToRegister(
		instruction asm.Instruction,
		shiftedSourceReg asm.Register,
		shiftNum asm.ConstantValue,
		srcReg, dstReg asm.Register,
	)

	// CompileSIMDByteToSIMDByte adds an instruction where source and destination operand is the SIMD register
	// specified as `srcReg.B8` and `dstReg.B8` where `.B8` part of register is called "arrangement".
	// See https://stackoverflow.com/questions/57294672/what-is-arrangement-specifier-16b-8b-in-arm-assembly-language-instructions
	//
	// TODO: implement this in CompileVectorRegisterToVectorRegister.
	CompileSIMDByteToSIMDByte(instruction asm.Instruction, srcReg, dstReg asm.Register)

	// CompileTwoSIMDBytesToSIMDByteRegister adds an instruction where source operand is two SIMD registers specified as `srcReg1.B8`,
	// and `srcReg2.B8` and the destination is the one SIMD register `dstReg.B8`.
	CompileTwoSIMDBytesToSIMDByteRegister(instruction asm.Instruction, srcReg1, srcReg2, dstReg asm.Register)

	// CompileSIMDByteToRegister adds an instruction where source operand is the SIMD register specified as `srcReg.B8`,
	// and the destination is the register `dstReg`.
	CompileSIMDByteToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register)

	// CompileConditionalRegisterSet adds an instruction to set 1 on dstReg if the condition satisfies,
	// otherwise set 0.
	CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, dstReg asm.Register)
	// CompileMemoryToVectorRegister TODO
	CompileMemoryToVectorRegister(instruction asm.Instruction, srcOffsetReg, dstReg asm.Register, arrangement VectorArrangement)
	// CompileVectorRegisterToMemory TODO
	CompileVectorRegisterToMemory(instruction asm.Instruction, srcReg, dstOffsetReg asm.Register, arrangement VectorArrangement)
	// CompileRegisterToVectorRegister TODO
	CompileRegisterToVectorRegister(instruction asm.Instruction, srcReg, dstReg asm.Register,
		arrangement VectorArrangement, index VectorIndex)
	// CompileVectorRegisterToVectorRegister TODO
	CompileVectorRegisterToVectorRegister(instruction asm.Instruction, srcReg, dstReg asm.Register, arrangement VectorArrangement)
}
