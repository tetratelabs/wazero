package arm64debug

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/heeus/hwazero/internal/asm"
	asm_arm64 "github.com/heeus/hwazero/internal/asm/arm64"
	"github.com/heeus/hwazero/internal/asm/golang_asm"
)

// NewDebugAssembler can be used for ensuring that our assembler produces exactly the same binary as Go.
// Disabled by default, but assigning this to NewAssembler allows us to debug assembler's bug.
//
// TODO: this will be removed after golang-asm removal.
// Note: this is intentionally exported in order to suppress bunch of "unused" lint errors on this function, testAssembler and testNode.
func NewDebugAssembler(temporaryRegister asm.Register) (asm_arm64.Assembler, error) {
	goasm, err := newAssembler(temporaryRegister)
	if err != nil {
		return nil, err
	}
	a := asm_arm64.NewAssemblerImpl(temporaryRegister)
	return &testAssembler{a: a, goasm: goasm}, nil
}

// testAssembler implements Assembler.
// This assembler ensures that our assembler produces exactly the same binary as the Go's official assembler.
// Disabled by default, and can be used for debugging only.
//
// TODO: this will be removed after golang-asm removal.
type testAssembler struct {
	goasm *assemblerGoAsmImpl
	a     *asm_arm64.AssemblerImpl
}

// testNode implements asm.Node for the usage with testAssembler.
//
// TODO: this will be removed after golang-asm removal.
type testNode struct {
	n     *asm_arm64.NodeImpl
	goasm *golang_asm.GolangAsmNode
}

// String implements fmt.Stringer.
func (tn *testNode) String() string {
	return tn.n.String()
}

// AssignJumpTarget implements the same method as documented on asm.Node.
func (tn *testNode) AssignJumpTarget(target asm.Node) {
	targetTestNode := target.(*testNode)
	tn.goasm.AssignJumpTarget(targetTestNode.goasm)
	tn.n.AssignJumpTarget(targetTestNode.n)
}

// AssignDestinationConstant implements the same method as documented on asm.Node.
func (tn *testNode) AssignDestinationConstant(value asm.ConstantValue) {
	tn.goasm.AssignDestinationConstant(value)
	tn.n.AssignDestinationConstant(value)
}

// AssignSourceConstant implements the same method as documented on asm.Node.
func (tn *testNode) AssignSourceConstant(value asm.ConstantValue) {
	tn.goasm.AssignSourceConstant(value)
	tn.n.AssignSourceConstant(value)
}

// OffsetInBinary implements the same method as documented on asm.Node.
func (tn *testNode) OffsetInBinary() asm.NodeOffsetInBinary {
	return tn.goasm.OffsetInBinary()
}

// Assemble implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) Assemble() ([]byte, error) {
	ret, err := ta.goasm.Assemble()
	if err != nil {
		return nil, err
	}

	a, err := ta.a.Assemble()
	if err != nil {
		return nil, fmt.Errorf("homemade assembler failed: %w", err)
	}

	if !bytes.Equal(ret, a) {
		expected := hex.EncodeToString(ret)
		actual := hex.EncodeToString(a)
		return nil, fmt.Errorf("expected (len=%d): %s\nactual(len=%d): %s", len(expected), expected, len(actual), actual)
	}
	return ret, nil
}

// SetJumpTargetOnNext implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) SetJumpTargetOnNext(nodes ...asm.Node) {
	for _, n := range nodes {
		targetTestNode := n.(*testNode)
		ta.goasm.SetJumpTargetOnNext(targetTestNode.goasm)
		ta.a.SetJumpTargetOnNext(targetTestNode.n)
	}
}

// BuildJumpTable implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) BuildJumpTable(table []byte, initialInstructions []asm.Node) {
	ta.goasm.BuildJumpTable(table, initialInstructions)
	ta.a.BuildJumpTable(table, initialInstructions)
}

// CompileStandAlone implements Assembler.CompileStandAlone.
func (ta *testAssembler) CompileStandAlone(instruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileStandAlone(instruction)
	ret2 := ta.a.CompileStandAlone(instruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_arm64.NodeImpl)}
}

// CompileConstToRegister implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileConstToRegister(
	instruction asm.Instruction,
	value asm.ConstantValue,
	destinationReg asm.Register,
) asm.Node {
	ret := ta.goasm.CompileConstToRegister(instruction, value, destinationReg)
	ret2 := ta.a.CompileConstToRegister(instruction, value, destinationReg)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_arm64.NodeImpl)}
}

// CompileRegisterToRegister implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	ta.goasm.CompileRegisterToRegister(instruction, from, to)
	ta.a.CompileRegisterToRegister(instruction, from, to)
}

// CompileMemoryToRegister implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileMemoryToRegister(
	instruction asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	destinationReg asm.Register,
) {
	ta.goasm.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
	ta.a.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
}

// CompileRegisterToMemory implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileRegisterToMemory(
	instruction asm.Instruction,
	sourceRegister, destinationBaseRegister asm.Register,
	destinationOffsetConst asm.ConstantValue,
) {
	ta.goasm.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
	ta.a.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
}

// CompileJump implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileJump(jmpInstruction)
	ret2 := ta.a.CompileJump(jmpInstruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_arm64.NodeImpl)}
}

// CompileJumpToMemory implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register) {
	ta.goasm.CompileJumpToMemory(jmpInstruction, baseReg)
	ta.a.CompileJumpToMemory(jmpInstruction, baseReg)
}

// CompileJumpToRegister implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	ta.goasm.CompileJumpToRegister(jmpInstruction, reg)
	ta.a.CompileJumpToRegister(jmpInstruction, reg)
}

// CompileReadInstructionAddress implements the same method as documented on asm_arm64.Assembler.
func (ta *testAssembler) CompileReadInstructionAddress(
	destinationRegister asm.Register,
	beforeAcquisitionTargetInstruction asm.Instruction,
) {
	ta.goasm.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
	ta.a.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
}

func (ta *testAssembler) CompileMemoryWithRegisterOffsetToRegister(
	instruction asm.Instruction,
	srcBaseReg, srcOffsetReg, dstReg asm.Register,
) {
	ta.goasm.CompileMemoryWithRegisterOffsetToRegister(instruction, srcBaseReg, srcOffsetReg, dstReg)
	ta.a.CompileMemoryWithRegisterOffsetToRegister(instruction, srcBaseReg, srcOffsetReg, dstReg)
}

func (ta *testAssembler) CompileRegisterToMemoryWithRegisterOffset(
	instruction asm.Instruction,
	srcReg, dstBaseReg, dstOffsetReg asm.Register,
) {
	ta.goasm.CompileRegisterToMemoryWithRegisterOffset(instruction, srcReg, dstBaseReg, dstOffsetReg)
	ta.a.CompileRegisterToMemoryWithRegisterOffset(instruction, srcReg, dstBaseReg, dstOffsetReg)
}

func (ta *testAssembler) CompileTwoRegistersToRegister(instruction asm.Instruction, src1, src2, dst asm.Register) {
	ta.goasm.CompileTwoRegistersToRegister(instruction, src1, src2, dst)
	ta.a.CompileTwoRegistersToRegister(instruction, src1, src2, dst)
}

func (ta *testAssembler) CompileThreeRegistersToRegister(
	instruction asm.Instruction,
	src1, src2, dst1, dst2 asm.Register,
) {
	ta.goasm.CompileThreeRegistersToRegister(instruction, src1, src2, dst1, dst2)
	ta.a.CompileThreeRegistersToRegister(instruction, src1, src2, dst1, dst2)
}

func (ta *testAssembler) CompileTwoRegistersToNone(instruction asm.Instruction, src1, src2 asm.Register) {
	ta.goasm.CompileTwoRegistersToNone(instruction, src1, src2)
	ta.a.CompileTwoRegistersToNone(instruction, src1, src2)
}

func (ta *testAssembler) CompileRegisterAndConstToNone(
	instruction asm.Instruction,
	src asm.Register,
	srcConst asm.ConstantValue,
) {
	ta.goasm.CompileRegisterAndConstToNone(instruction, src, srcConst)
	ta.a.CompileRegisterAndConstToNone(instruction, src, srcConst)
}

func (ta *testAssembler) CompileLeftShiftedRegisterToRegister(
	instruction asm.Instruction,
	shiftedSourceReg asm.Register,
	shiftNum asm.ConstantValue,
	srcReg, dstReg asm.Register,
) {
	ta.goasm.CompileLeftShiftedRegisterToRegister(instruction, shiftedSourceReg, shiftNum, srcReg, dstReg)
	ta.a.CompileLeftShiftedRegisterToRegister(instruction, shiftedSourceReg, shiftNum, srcReg, dstReg)
}

func (ta *testAssembler) CompileSIMDByteToSIMDByte(instruction asm.Instruction, srcReg, dstReg asm.Register) {
	ta.goasm.CompileSIMDByteToSIMDByte(instruction, srcReg, dstReg)
	ta.a.CompileSIMDByteToSIMDByte(instruction, srcReg, dstReg)
}

func (ta *testAssembler) CompileTwoSIMDBytesToSIMDByteRegister(
	instruction asm.Instruction,
	srcReg1, srcReg2, dstReg asm.Register,
) {
	ta.goasm.CompileTwoSIMDBytesToSIMDByteRegister(instruction, srcReg1, srcReg2, dstReg)
	ta.a.CompileTwoSIMDBytesToSIMDByteRegister(instruction, srcReg1, srcReg2, dstReg)
}

func (ta *testAssembler) CompileSIMDByteToRegister(instruction asm.Instruction, srcReg, dstReg asm.Register) {
	ta.goasm.CompileSIMDByteToRegister(instruction, srcReg, dstReg)
	ta.a.CompileSIMDByteToRegister(instruction, srcReg, dstReg)
}

func (ta *testAssembler) CompileConditionalRegisterSet(cond asm.ConditionalRegisterState, dstReg asm.Register) {
	ta.goasm.CompileConditionalRegisterSet(cond, dstReg)
	ta.a.CompileConditionalRegisterSet(cond, dstReg)
}
