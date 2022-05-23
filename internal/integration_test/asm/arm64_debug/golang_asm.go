package arm64debug

import (
	"fmt"
	"math"

	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/internal/asm"
	asm_arm64 "github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/integration_test/asm/golang_asm"
)

// newAssembler implements asm.NewAssembler by golang-asm.
func newAssembler(temporaryRegister asm.Register) (*assemblerGoAsmImpl, error) {
	g, err := golang_asm.NewGolangAsmBaseAssembler("arm64")
	return &assemblerGoAsmImpl{GolangAsmBaseAssembler: g, temporaryRegister: temporaryRegister}, err
}

// assemblerGoAsmImpl implements asm_arm64.Assembler for golang-asm library.
type assemblerGoAsmImpl struct {
	*golang_asm.GolangAsmBaseAssembler
	temporaryRegister asm.Register
}

// CompileConstToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileConstToRegister(instruction asm.Instruction, constValue asm.ConstantValue, destinationReg asm.Register) asm.Node {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	if constValue == 0 {
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = arm64.REGZERO
	} else {
		inst.From.Type = obj.TYPE_CONST
		// Note: in raw arm64 assembly, immediates larger than 16-bits
		// are not supported, but the assembler takes care of this and
		// emits corresponding (at most) 4-instructions to load such large constants.
		inst.From.Offset = constValue
	}

	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(inst)
	return golang_asm.NewGolangAsmNode(inst)
}

// CompileMemoryToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst asm.ConstantValue, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	inst.From.Offset = sourceOffsetConst
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(inst)
}

// CompileMemoryWithRegisterOffsetToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileMemoryWithRegisterOffsetToRegister(instruction asm.Instruction, sourceBaseReg, sourceOffsetReg, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = castAsGolangAsmRegister[sourceBaseReg]
	inst.From.Index = castAsGolangAsmRegister[sourceOffsetReg]
	inst.From.Scale = 1
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(inst)
}

// CompileRegisterToMemory implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToMemory(instruction asm.Instruction, sourceReg asm.Register, destinationBaseReg asm.Register, destinationOffsetConst asm.ConstantValue) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = castAsGolangAsmRegister[destinationBaseReg]
	inst.To.Offset = destinationOffsetConst
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[sourceReg]
	a.AddInstruction(inst)
}

// CompileRegisterToMemoryWithRegisterOffset implements Assembler.CompileRegisterToMemoryWithRegisterOffset.
func (a *assemblerGoAsmImpl) CompileRegisterToMemoryWithRegisterOffset(instruction asm.Instruction, sourceReg, destinationBaseReg, destinationOffsetReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = castAsGolangAsmRegister[destinationBaseReg]
	inst.To.Index = castAsGolangAsmRegister[destinationOffsetReg]
	inst.To.Scale = 1
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[sourceReg]
	a.AddInstruction(inst)
}

// CompileRegisterToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[to]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[from]
	a.AddInstruction(inst)
}

// CompileTwoRegistersToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileTwoRegistersToRegister(instruction asm.Instruction, src1, src2, destination asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destination]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	a.AddInstruction(inst)
}

// CompileThreeRegistersToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileThreeRegistersToRegister(instruction asm.Instruction, src1, src2, src3, dst asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[dst]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	inst.RestArgs = append(inst.RestArgs, obj.Addr{Type: obj.TYPE_REG, Reg: castAsGolangAsmRegister[src3]})
	a.AddInstruction(inst)
}

// CompileTwoRegistersToNone implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileTwoRegistersToNone(instruction asm.Instruction, src1, src2 asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[src1]
	inst.Reg = castAsGolangAsmRegister[src2]
	a.AddInstruction(inst)
}

// CompileRegisterAndConstToNone implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileRegisterAndConstToNone(instruction asm.Instruction, src asm.Register, srcConst asm.ConstantValue) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_CONST
	inst.From.Offset = srcConst
	inst.Reg = castAsGolangAsmRegister[src]
	a.AddInstruction(inst)
}

// CompileJump implements the same method as documented on asm.AssemblerBase.
func (a *assemblerGoAsmImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	br := a.NewProg()
	br.As = castAsGolangAsmInstruction[jmpInstruction]
	br.To.Type = obj.TYPE_BRANCH
	a.AddInstruction(br)
	return golang_asm.NewGolangAsmNode(br)
}

// CompileJumpToMemory implements the same method as documented on asm.AssemblerBase.
func (a *assemblerGoAsmImpl) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register) {
	br := a.NewProg()
	br.As = castAsGolangAsmInstruction[jmpInstruction]
	br.To.Type = obj.TYPE_MEM
	br.To.Reg = castAsGolangAsmRegister[baseReg]
	a.AddInstruction(br)
}

// CompileJumpToRegister implements the same method as documented on asm.AssemblerBase.
func (a *assemblerGoAsmImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	ret := a.NewProg()
	ret.As = castAsGolangAsmInstruction[jmpInstruction]
	ret.To.Type = obj.TYPE_REG
	ret.To.Reg = castAsGolangAsmRegister[reg]
	a.AddInstruction(ret)
}

// CompileStandAlone implements the same method as documented on asm.AssemblerBase.
func (a *assemblerGoAsmImpl) CompileStandAlone(instruction asm.Instruction) asm.Node {
	prog := a.NewProg()
	prog.As = castAsGolangAsmInstruction[instruction]
	a.AddInstruction(prog)
	return golang_asm.NewGolangAsmNode(prog)
}

// CompileLeftShiftedRegisterToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileLeftShiftedRegisterToRegister(instruction asm.Instruction, shiftedSourceReg asm.Register, shiftNum asm.ConstantValue, srcReg, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	// See https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L120-L131
	inst.From.Type = obj.TYPE_SHIFT
	inst.From.Offset = (int64(castAsGolangAsmRegister[shiftedSourceReg])&31)<<16 | 0<<22 | (shiftNum&63)<<10
	inst.Reg = castAsGolangAsmRegister[srcReg]
	a.AddInstruction(inst)
}

// CompileReadInstructionAddress implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileReadInstructionAddress(destinationReg asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	// Emit ADR instruction to read the specified instruction's absolute address.
	// Note: we cannot emit the "ADR REG, $(target's offset from here)" due to the
	// incapability of the assembler. Instead, we emit "ADR REG, ." meaning that
	// "reading the current program counter" = "reading the absolute address of this ADR instruction".
	// And then, after compilation phase, we directly edit the native code slice so that
	// it can properly read the target instruction's absolute address.
	readAddress := a.NewProg()
	readAddress.As = arm64.AADR
	readAddress.From.Type = obj.TYPE_BRANCH
	readAddress.To.Type = obj.TYPE_REG
	readAddress.To.Reg = castAsGolangAsmRegister[destinationReg]
	a.AddInstruction(readAddress)

	// Setup the callback to modify the instruction bytes after compilation.
	// Note: this is the closure over readAddress (*obj.Prog).
	a.AddOnGenerateCallBack(func(code []byte) error {
		// Find the target instruction.
		target := readAddress
		beforeTarget := castAsGolangAsmInstruction[beforeAcquisitionTargetInstruction]
		for target != nil {
			if target.As == beforeTarget {
				// At this point, target is the instruction right before the target instruction.
				// Thus, advance one more time to make target the target instruction.
				target = target.Link
				break
			}
			target = target.Link
		}

		if target == nil {
			return fmt.Errorf("BUG: target instruction not %s found for read instruction address", asm_arm64.InstructionName(beforeAcquisitionTargetInstruction))
		}

		offset := target.Pc - readAddress.Pc
		if offset > math.MaxUint8 {
			// We could support up to 20-bit integer, but byte should be enough for our impl.
			// If the necessity comes up, we could fix the below to support larger offsets.
			return fmt.Errorf("BUG: too large offset for read")
		}

		v := byte(offset)
		adrInst := code[readAddress.Pc : readAddress.Pc+4]
		// According to the binary format of ADR instruction in arm64:
		// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/ADR--Form-PC-relative-address-?lang=en
		//
		// The 0 to 1 bits live on 29 to 30 bits of the instruction.
		adrInst[3] |= (v & 0b00000011) << 5
		// The 2 to 4 bits live on 5 to 7 bits of the instruction.
		adrInst[0] |= (v & 0b00011100) << 3
		// The 5 to 7 bits live on 8 to 10 bits of the instruction.
		adrInst[1] |= (v & 0b11100000) >> 5
		return nil
	})
}

// CompileConditionalRegisterSet implements the same method as documented on asm_arm64.Assembler.
//
// We use CSET instruction to set 1 on the register if the condition satisfies:
// https://developer.arm.com/documentation/100076/0100/a64-instruction-set-reference/a64-general-instructions/cset
func (a *assemblerGoAsmImpl) CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, destinationReg asm.Register) {
	inst := a.NewProg()
	inst.As = arm64.ACSET
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = castAsGolangAsmRegister[destinationReg]
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmConditionalRegister[cond]
	a.AddInstruction(inst)
}

// simdRegisterForScalarFloatRegister returns SIMD register which corresponds to the given scalar float register.
// In other words, this returns: REG_F0 -> RegV0, REG_F1 -> RegV1, ...., REG_F31 -> RegV31.
func simdRegisterForScalarFloatRegister(freg int16) int16 {
	return freg + (arm64.REG_F31 - arm64.REG_F0) + 1
}

// CompileTwoSIMDBytesToSIMDByteRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileTwoSIMDBytesToSIMDByteRegister(instruction asm.Instruction, srcReg1, srcReg2, dstReg asm.Register) {
	src1FloatReg, src2FloatReg, dstFloatReg := castAsGolangAsmRegister[srcReg1], castAsGolangAsmRegister[srcReg2], castAsGolangAsmRegister[dstReg]
	src1VReg, src2VReg, dstVReg := simdRegisterForScalarFloatRegister(src1FloatReg), simdRegisterForScalarFloatRegister(src2FloatReg), simdRegisterForScalarFloatRegister(dstFloatReg)

	// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
	// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = dstVReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = src1VReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	inst.Reg = src2VReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	a.AddInstruction(inst)

}

// CompileSIMDByteToSIMDByte implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileSIMDByteToSIMDByte(instruction asm.Instruction, srcReg, dstReg asm.Register) {
	srcFloatReg, dstFloatReg := castAsGolangAsmRegister[srcReg], castAsGolangAsmRegister[dstReg]
	srcVReg, dstVReg := simdRegisterForScalarFloatRegister(srcFloatReg), simdRegisterForScalarFloatRegister(dstFloatReg)

	// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
	// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = dstVReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = srcVReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	a.AddInstruction(inst)
}

// CompileSIMDByteToRegister implements the same method as documented on asm_arm64.Assembler.
func (a *assemblerGoAsmImpl) CompileSIMDByteToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register) {
	srcFloatReg, dstFlaotReg := castAsGolangAsmRegister[srcReg], castAsGolangAsmRegister[dstReg]
	srcVReg, dstVReg := simdRegisterForScalarFloatRegister(srcFloatReg), simdRegisterForScalarFloatRegister(dstFlaotReg)

	// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
	// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = dstVReg
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = srcVReg&31 + arm64.REG_ARNG + (arm64.ARNG_8B&15)<<5
	a.AddInstruction(inst)
}

func createOffsetForVectorRegList(arr asm_arm64.VectorArrangement, reg asm.Register) (offset int64) {
	// https://github.com/golang/go/blob/19309779ac5e2f5a2fd3cbb34421dafb2855ac21/src/cmd/asm/internal/arch/arm64.go#L372
	// https://github.com/golang/go/blob/19309779ac5e2f5a2fd3cbb34421dafb2855ac21/src/cmd/asm/internal/arch/arm64.go#L332
	// https://github.com/golang/go/blob/19309779ac5e2f5a2fd3cbb34421dafb2855ac21/src/cmd/asm/internal/asm/parse.go#L1143-L1148
	var curQ, curSize int64
	switch arr {
	case asm_arm64.VectorArrangement8B:
		curSize = 0
		curQ = 0
	case asm_arm64.VectorArrangement16B:
		curSize = 0
		curQ = 1
	case asm_arm64.VectorArrangement4H:
		curSize = 1
		curQ = 0
	case asm_arm64.VectorArrangement8H:
		curSize = 1
		curQ = 1
	case asm_arm64.VectorArrangement2S:
		curSize = 2
		curQ = 0
	case asm_arm64.VectorArrangement4S:
		curSize = 2
		curQ = 1
	case asm_arm64.VectorArrangement1D:
		curSize = 3
		curQ = 0
	case asm_arm64.VectorArrangement2D:
		curSize = 3
		curQ = 1
	}
	return (int64(curQ) & 1 << 30) | ((curSize & 3) << 10) | (0x7 << 12) | 1<<60 | int64(castAsGolangAsVectorRegister[reg]&31)
}

func (a *assemblerGoAsmImpl) CompileMemoryToVectorRegister(
	instruction asm.Instruction, srcOffsetReg, dstReg asm.Register, arrangement asm_arm64.VectorArrangement,
) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REGLIST
	inst.To.Offset = createOffsetForVectorRegList(arrangement, dstReg)
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = castAsGolangAsmRegister[srcOffsetReg]
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileVectorRegisterToMemory(instruction asm.Instruction, srcReg, dstOffsetReg asm.Register,
	arrangement asm_arm64.VectorArrangement) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = castAsGolangAsmRegister[dstOffsetReg]
	inst.From.Type = obj.TYPE_REGLIST
	inst.From.Offset = createOffsetForVectorRegList(arrangement, srcReg)
	a.AddInstruction(inst)
}

func (a *assemblerGoAsmImpl) CompileRegisterToVectorRegister(instruction asm.Instruction, srcReg, dstReg asm.Register,
	arrangement asm_arm64.VectorArrangement, index asm_arm64.VectorIndex) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = (castAsGolangAsVectorRegister[dstReg] & 31) + arm64.REG_ELEM +
		(castAsGolangAsmArrangement[arrangement]&15)<<5
	inst.To.Index = int16(index)
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = castAsGolangAsmRegister[srcReg]
	a.AddInstruction(inst)
}
func (a *assemblerGoAsmImpl) CompileVectorRegisterToVectorRegister(instruction asm.Instruction, srcReg, dstReg asm.Register, arrangement asm_arm64.VectorArrangement) {
	inst := a.NewProg()
	inst.As = castAsGolangAsmInstruction[instruction]

	switch instruction {
	case asm_arm64.VMOV:
		// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
		// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = castAsGolangAsVectorRegister[dstReg]&31 + arm64.REG_ARNG + (castAsGolangAsmArrangement[arrangement]&15)<<5
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = castAsGolangAsVectorRegister[srcReg]&31 + arm64.REG_ARNG + (castAsGolangAsmArrangement[arrangement]&15)<<5
		a.AddInstruction(inst)
	case asm_arm64.VADD:
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = castAsGolangAsVectorRegister[dstReg]&31 + arm64.REG_ARNG + (castAsGolangAsmArrangement[arrangement]&15)<<5
		inst.Reg = castAsGolangAsVectorRegister[srcReg]&31 + arm64.REG_ARNG + (castAsGolangAsmArrangement[arrangement]&15)<<5
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = castAsGolangAsVectorRegister[dstReg]&31 + arm64.REG_ARNG + (castAsGolangAsmArrangement[arrangement]&15)<<5
		a.AddInstruction(inst)
	case asm_arm64.VFADDD:
		panic("Unsupported in golang-asm")
	case asm_arm64.VFADDS:
		panic("Unsupported in golang-asm")
	}
}

var castAsGolangAsmArrangement = [...]int16{
	asm_arm64.VectorArrangement1D:  arm64.ARNG_1D,
	asm_arm64.VectorArrangement2D:  arm64.ARNG_2D,
	asm_arm64.VectorArrangement2S:  arm64.ARNG_2S,
	asm_arm64.VectorArrangement4S:  arm64.ARNG_4S,
	asm_arm64.VectorArrangement8H:  arm64.ARNG_8H,
	asm_arm64.VectorArrangement4H:  arm64.ARNG_4H,
	asm_arm64.VectorArrangement16B: arm64.ARNG_16B,
	asm_arm64.VectorArrangement8B:  arm64.ARNG_8B,
	asm_arm64.VectorArrangementB:   arm64.ARNG_B,
	asm_arm64.VectorArrangementH:   arm64.ARNG_H,
	asm_arm64.VectorArrangementS:   arm64.ARNG_S,
	asm_arm64.VectorArrangementD:   arm64.ARNG_D,
}

// castAsGolangAsmConditionalRegister maps the conditional states to golang-asm specific conditional state register values.
var castAsGolangAsmConditionalRegister = [...]int16{
	asm_arm64.CondEQ: arm64.COND_EQ,
	asm_arm64.CondNE: arm64.COND_NE,
	asm_arm64.CondHS: arm64.COND_HS,
	asm_arm64.CondLO: arm64.COND_LO,
	asm_arm64.CondMI: arm64.COND_MI,
	asm_arm64.CondPL: arm64.COND_PL,
	asm_arm64.CondVS: arm64.COND_VS,
	asm_arm64.CondVC: arm64.COND_VC,
	asm_arm64.CondHI: arm64.COND_HI,
	asm_arm64.CondLS: arm64.COND_LS,
	asm_arm64.CondGE: arm64.COND_GE,
	asm_arm64.CondLT: arm64.COND_LT,
	asm_arm64.CondGT: arm64.COND_GT,
	asm_arm64.CondLE: arm64.COND_LE,
	asm_arm64.CondAL: arm64.COND_AL,
	asm_arm64.CondNV: arm64.COND_NV,
}

var castAsGolangAsVectorRegister = [...]int16{
	asm_arm64.RegV0:  arm64.REG_V0,
	asm_arm64.RegV1:  arm64.REG_V1,
	asm_arm64.RegV2:  arm64.REG_V2,
	asm_arm64.RegV3:  arm64.REG_V3,
	asm_arm64.RegV4:  arm64.REG_V4,
	asm_arm64.RegV5:  arm64.REG_V5,
	asm_arm64.RegV6:  arm64.REG_V6,
	asm_arm64.RegV7:  arm64.REG_V7,
	asm_arm64.RegV8:  arm64.REG_V8,
	asm_arm64.RegV9:  arm64.REG_V9,
	asm_arm64.RegV10: arm64.REG_V10,
	asm_arm64.RegV11: arm64.REG_V11,
	asm_arm64.RegV12: arm64.REG_V12,
	asm_arm64.RegV13: arm64.REG_V13,
	asm_arm64.RegV14: arm64.REG_V14,
	asm_arm64.RegV15: arm64.REG_V15,
	asm_arm64.RegV16: arm64.REG_V16,
	asm_arm64.RegV17: arm64.REG_V17,
	asm_arm64.RegV18: arm64.REG_V18,
	asm_arm64.RegV19: arm64.REG_V19,
	asm_arm64.RegV20: arm64.REG_V20,
	asm_arm64.RegV21: arm64.REG_V21,
	asm_arm64.RegV22: arm64.REG_V22,
	asm_arm64.RegV23: arm64.REG_V23,
	asm_arm64.RegV24: arm64.REG_V24,
	asm_arm64.RegV25: arm64.REG_V25,
	asm_arm64.RegV26: arm64.REG_V26,
	asm_arm64.RegV27: arm64.REG_V27,
	asm_arm64.RegV28: arm64.REG_V28,
	asm_arm64.RegV29: arm64.REG_V29,
	asm_arm64.RegV30: arm64.REG_V30,
	asm_arm64.RegV31: arm64.REG_V31,
}

// castAsGolangAsmRegister maps the registers to golang-asm specific registers values.
var castAsGolangAsmRegister = [...]int16{
	asm_arm64.RegR0:   arm64.REG_R0,
	asm_arm64.RegR1:   arm64.REG_R1,
	asm_arm64.RegR2:   arm64.REG_R2,
	asm_arm64.RegR3:   arm64.REG_R3,
	asm_arm64.RegR4:   arm64.REG_R4,
	asm_arm64.RegR5:   arm64.REG_R5,
	asm_arm64.RegR6:   arm64.REG_R6,
	asm_arm64.RegR7:   arm64.REG_R7,
	asm_arm64.RegR8:   arm64.REG_R8,
	asm_arm64.RegR9:   arm64.REG_R9,
	asm_arm64.RegR10:  arm64.REG_R10,
	asm_arm64.RegR11:  arm64.REG_R11,
	asm_arm64.RegR12:  arm64.REG_R12,
	asm_arm64.RegR13:  arm64.REG_R13,
	asm_arm64.RegR14:  arm64.REG_R14,
	asm_arm64.RegR15:  arm64.REG_R15,
	asm_arm64.RegR16:  arm64.REG_R16,
	asm_arm64.RegR17:  arm64.REG_R17,
	asm_arm64.RegR18:  arm64.REG_R18,
	asm_arm64.RegR19:  arm64.REG_R19,
	asm_arm64.RegR20:  arm64.REG_R20,
	asm_arm64.RegR21:  arm64.REG_R21,
	asm_arm64.RegR22:  arm64.REG_R22,
	asm_arm64.RegR23:  arm64.REG_R23,
	asm_arm64.RegR24:  arm64.REG_R24,
	asm_arm64.RegR25:  arm64.REG_R25,
	asm_arm64.RegR26:  arm64.REG_R26,
	asm_arm64.RegR27:  arm64.REG_R27,
	asm_arm64.RegR28:  arm64.REG_R28,
	asm_arm64.RegR29:  arm64.REG_R29,
	asm_arm64.RegR30:  arm64.REG_R30,
	asm_arm64.RegZERO: arm64.REGZERO,
	asm_arm64.RegV0:   arm64.REG_F0,
	asm_arm64.RegV1:   arm64.REG_F1,
	asm_arm64.RegV2:   arm64.REG_F2,
	asm_arm64.RegV3:   arm64.REG_F3,
	asm_arm64.RegV4:   arm64.REG_F4,
	asm_arm64.RegV5:   arm64.REG_F5,
	asm_arm64.RegV6:   arm64.REG_F6,
	asm_arm64.RegV7:   arm64.REG_F7,
	asm_arm64.RegV8:   arm64.REG_F8,
	asm_arm64.RegV9:   arm64.REG_F9,
	asm_arm64.RegV10:  arm64.REG_F10,
	asm_arm64.RegV11:  arm64.REG_F11,
	asm_arm64.RegV12:  arm64.REG_F12,
	asm_arm64.RegV13:  arm64.REG_F13,
	asm_arm64.RegV14:  arm64.REG_F14,
	asm_arm64.RegV15:  arm64.REG_F15,
	asm_arm64.RegV16:  arm64.REG_F16,
	asm_arm64.RegV17:  arm64.REG_F17,
	asm_arm64.RegV18:  arm64.REG_F18,
	asm_arm64.RegV19:  arm64.REG_F19,
	asm_arm64.RegV20:  arm64.REG_F20,
	asm_arm64.RegV21:  arm64.REG_F21,
	asm_arm64.RegV22:  arm64.REG_F22,
	asm_arm64.RegV23:  arm64.REG_F23,
	asm_arm64.RegV24:  arm64.REG_F24,
	asm_arm64.RegV25:  arm64.REG_F25,
	asm_arm64.RegV26:  arm64.REG_F26,
	asm_arm64.RegV27:  arm64.REG_F27,
	asm_arm64.RegV28:  arm64.REG_F28,
	asm_arm64.RegV29:  arm64.REG_F29,
	asm_arm64.RegV30:  arm64.REG_F30,
	asm_arm64.RegV31:  arm64.REG_F31,
	asm_arm64.RegFPSR: arm64.REG_FPSR,
}

// castAsGolangAsmInstruction maps the instructions to golang-asm specific instructions values.
var castAsGolangAsmInstruction = [...]obj.As{
	asm_arm64.NOP:      obj.ANOP,
	asm_arm64.RET:      obj.ARET,
	asm_arm64.ADD:      arm64.AADD,
	asm_arm64.ADDS:     arm64.AADDS,
	asm_arm64.ADDW:     arm64.AADDW,
	asm_arm64.ADR:      arm64.AADR,
	asm_arm64.AND:      arm64.AAND,
	asm_arm64.ANDW:     arm64.AANDW,
	asm_arm64.ASR:      arm64.AASR,
	asm_arm64.ASRW:     arm64.AASRW,
	asm_arm64.B:        arm64.AB,
	asm_arm64.BEQ:      arm64.ABEQ,
	asm_arm64.BGE:      arm64.ABGE,
	asm_arm64.BGT:      arm64.ABGT,
	asm_arm64.BHI:      arm64.ABHI,
	asm_arm64.BHS:      arm64.ABHS,
	asm_arm64.BLE:      arm64.ABLE,
	asm_arm64.BLO:      arm64.ABLO,
	asm_arm64.BLS:      arm64.ABLS,
	asm_arm64.BLT:      arm64.ABLT,
	asm_arm64.BMI:      arm64.ABMI,
	asm_arm64.BPL:      arm64.ABPL,
	asm_arm64.BNE:      arm64.ABNE,
	asm_arm64.BVS:      arm64.ABVS,
	asm_arm64.CLZ:      arm64.ACLZ,
	asm_arm64.CLZW:     arm64.ACLZW,
	asm_arm64.CMP:      arm64.ACMP,
	asm_arm64.CMPW:     arm64.ACMPW,
	asm_arm64.CSET:     arm64.ACSET,
	asm_arm64.EOR:      arm64.AEOR,
	asm_arm64.EORW:     arm64.AEORW,
	asm_arm64.FABSD:    arm64.AFABSD,
	asm_arm64.FABSS:    arm64.AFABSS,
	asm_arm64.FADDD:    arm64.AFADDD,
	asm_arm64.FADDS:    arm64.AFADDS,
	asm_arm64.FCMPD:    arm64.AFCMPD,
	asm_arm64.FCMPS:    arm64.AFCMPS,
	asm_arm64.FCVTDS:   arm64.AFCVTDS,
	asm_arm64.FCVTSD:   arm64.AFCVTSD,
	asm_arm64.FCVTZSD:  arm64.AFCVTZSD,
	asm_arm64.FCVTZSDW: arm64.AFCVTZSDW,
	asm_arm64.FCVTZSS:  arm64.AFCVTZSS,
	asm_arm64.FCVTZSSW: arm64.AFCVTZSSW,
	asm_arm64.FCVTZUD:  arm64.AFCVTZUD,
	asm_arm64.FCVTZUDW: arm64.AFCVTZUDW,
	asm_arm64.FCVTZUS:  arm64.AFCVTZUS,
	asm_arm64.FCVTZUSW: arm64.AFCVTZUSW,
	asm_arm64.FDIVD:    arm64.AFDIVD,
	asm_arm64.FDIVS:    arm64.AFDIVS,
	asm_arm64.FMAXD:    arm64.AFMAXD,
	asm_arm64.FMAXS:    arm64.AFMAXS,
	asm_arm64.FMIND:    arm64.AFMIND,
	asm_arm64.FMINS:    arm64.AFMINS,
	asm_arm64.FMOVD:    arm64.AFMOVD,
	asm_arm64.FMOVS:    arm64.AFMOVS,
	asm_arm64.FMULD:    arm64.AFMULD,
	asm_arm64.FMULS:    arm64.AFMULS,
	asm_arm64.FNEGD:    arm64.AFNEGD,
	asm_arm64.FNEGS:    arm64.AFNEGS,
	asm_arm64.FRINTMD:  arm64.AFRINTMD,
	asm_arm64.FRINTMS:  arm64.AFRINTMS,
	asm_arm64.FRINTND:  arm64.AFRINTND,
	asm_arm64.FRINTNS:  arm64.AFRINTNS,
	asm_arm64.FRINTPD:  arm64.AFRINTPD,
	asm_arm64.FRINTPS:  arm64.AFRINTPS,
	asm_arm64.FRINTZD:  arm64.AFRINTZD,
	asm_arm64.FRINTZS:  arm64.AFRINTZS,
	asm_arm64.FSQRTD:   arm64.AFSQRTD,
	asm_arm64.FSQRTS:   arm64.AFSQRTS,
	asm_arm64.FSUBD:    arm64.AFSUBD,
	asm_arm64.FSUBS:    arm64.AFSUBS,
	asm_arm64.LSL:      arm64.ALSL,
	asm_arm64.LSLW:     arm64.ALSLW,
	asm_arm64.LSR:      arm64.ALSR,
	asm_arm64.LSRW:     arm64.ALSRW,
	asm_arm64.MOVB:     arm64.AMOVB,
	asm_arm64.MOVBU:    arm64.AMOVBU,
	asm_arm64.MOVD:     arm64.AMOVD,
	asm_arm64.MOVH:     arm64.AMOVH,
	asm_arm64.MOVHU:    arm64.AMOVHU,
	asm_arm64.MOVW:     arm64.AMOVW,
	asm_arm64.MOVWU:    arm64.AMOVWU,
	asm_arm64.MRS:      arm64.AMRS,
	asm_arm64.MSR:      arm64.AMSR,
	asm_arm64.MSUB:     arm64.AMSUB,
	asm_arm64.MSUBW:    arm64.AMSUBW,
	asm_arm64.MUL:      arm64.AMUL,
	asm_arm64.MULW:     arm64.AMULW,
	asm_arm64.NEG:      arm64.ANEG,
	asm_arm64.NEGW:     arm64.ANEGW,
	asm_arm64.ORR:      arm64.AORR,
	asm_arm64.ORRW:     arm64.AORRW,
	asm_arm64.RBIT:     arm64.ARBIT,
	asm_arm64.RBITW:    arm64.ARBITW,
	asm_arm64.ROR:      arm64.AROR,
	asm_arm64.RORW:     arm64.ARORW,
	asm_arm64.SCVTFD:   arm64.ASCVTFD,
	asm_arm64.SCVTFS:   arm64.ASCVTFS,
	asm_arm64.SCVTFWD:  arm64.ASCVTFWD,
	asm_arm64.SCVTFWS:  arm64.ASCVTFWS,
	asm_arm64.SDIV:     arm64.ASDIV,
	asm_arm64.SDIVW:    arm64.ASDIVW,
	asm_arm64.SUB:      arm64.ASUB,
	asm_arm64.SUBS:     arm64.ASUBS,
	asm_arm64.SUBW:     arm64.ASUBW,
	asm_arm64.SXTB:     arm64.ASXTB,
	asm_arm64.SXTBW:    arm64.ASXTBW,
	asm_arm64.SXTH:     arm64.ASXTH,
	asm_arm64.SXTHW:    arm64.ASXTHW,
	asm_arm64.SXTW:     arm64.ASXTW,
	asm_arm64.UCVTFD:   arm64.AUCVTFD,
	asm_arm64.UCVTFS:   arm64.AUCVTFS,
	asm_arm64.UCVTFWD:  arm64.AUCVTFWD,
	asm_arm64.UCVTFWS:  arm64.AUCVTFWS,
	asm_arm64.UDIV:     arm64.AUDIV,
	asm_arm64.UDIVW:    arm64.AUDIVW,
	asm_arm64.VBIT:     arm64.AVBIT,
	asm_arm64.VCNT:     arm64.AVCNT,
	asm_arm64.VUADDLV:  arm64.AVUADDLV,
	asm_arm64.VMOV:     arm64.AVMOV,
	asm_arm64.VLD1:     arm64.AVLD1,
	asm_arm64.VST1:     arm64.AVST1,
	asm_arm64.VADD:     arm64.AVADD,
}
