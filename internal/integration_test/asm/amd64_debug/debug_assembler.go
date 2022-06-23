package amd64_debug

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/objabi"

	"github.com/tetratelabs/wazero/internal/asm"
	asm_amd64 "github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/integration_test/asm/golang_asm"
)

// NewDebugAssembler can be used for ensuring that our assembler produces exactly the same binary as Go.
// Disabled by default, but assigning this to NewAssembler allows us to debug assembler's bug.
//
// Note: this will be removed after golang-asm removal.
// Note: this is intentionally exported in order to suppress bunch of "unused" lint errors on this function, testAssembler and testNode.
func NewDebugAssembler() (asm_amd64.Assembler, error) {
	goasm, err := newGolangAsmAssembler()
	if err != nil {
		return nil, err
	}
	a := asm_amd64.NewAssemblerImpl()

	// If nop padding is enabled, it is really difficult to match the logics of golang-asm since it's so complex
	// and not well-documented. Given that NOP padding is just padding NOPs literally, and it doesn't affect
	// the semantics of program, we should be fine to debug without padding enabled.
	objabi.GOAMD64 = "disable"
	a.EnablePadding = false
	return &testAssembler{a: a, goasm: goasm}, nil
}

// testAssembler implements asm_amd64.Assembler.
// This assembler ensures that our assembler produces exactly the same binary as the Go's official assembler.
// Disabled by default, and can be used for debugging only.
//
// Note: this will be removed after golang-asm removal.
type testAssembler struct {
	goasm *assemblerGoAsmImpl
	a     *asm_amd64.AssemblerImpl
}

// testNode implements asm.Node for the usage with testAssembler.
//
// Note: this will be removed after golang-asm removal.
type testNode struct {
	n     *asm_amd64.NodeImpl
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

// Assemble implements the same method as documented on asm_amd64.Assembler.
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

// SetJumpTargetOnNext implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) SetJumpTargetOnNext(nodes ...asm.Node) {
	for _, n := range nodes {
		targetTestNode := n.(*testNode)
		ta.goasm.SetJumpTargetOnNext(targetTestNode.goasm)
		ta.a.SetJumpTargetOnNext(targetTestNode.n)
	}
}

// BuildJumpTable implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) BuildJumpTable(table *asm.StaticConst, initialInstructions []asm.Node) {
	panic("BuildJumpTable is not supported by golang-asm")
}

// CompileStandAlone implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileStandAlone(instruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileStandAlone(instruction)
	ret2 := ta.a.CompileStandAlone(instruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileConstToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileConstToRegister(instruction asm.Instruction, value asm.ConstantValue, destinationReg asm.Register) asm.Node {
	ret := ta.goasm.CompileConstToRegister(instruction, value, destinationReg)
	ret2 := ta.a.CompileConstToRegister(instruction, value, destinationReg)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileRegisterToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	ta.goasm.CompileRegisterToRegister(instruction, from, to)
	ta.a.CompileRegisterToRegister(instruction, from, to)
}

// CompileMemoryToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileMemoryToRegister(
	instruction asm.Instruction,
	sourceBaseReg asm.Register,
	sourceOffsetConst asm.ConstantValue,
	destinationReg asm.Register,
) {
	ta.goasm.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
	ta.a.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
}

// CompileRegisterToMemory implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToMemory(
	instruction asm.Instruction,
	sourceRegister, destinationBaseRegister asm.Register,
	destinationOffsetConst asm.ConstantValue,
) {
	ta.goasm.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
	ta.a.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
}

// CompileJump implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileJump(jmpInstruction)
	ret2 := ta.a.CompileJump(jmpInstruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileJumpToMemory implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileJumpToMemory(
	jmpInstruction asm.Instruction,
	baseReg asm.Register,
	offset asm.ConstantValue,
) {
	ta.goasm.CompileJumpToMemory(jmpInstruction, baseReg, offset)
	ta.a.CompileJumpToMemory(jmpInstruction, baseReg, offset)
}

// CompileJumpToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	ta.goasm.CompileJumpToRegister(jmpInstruction, reg)
	ta.a.CompileJumpToRegister(jmpInstruction, reg)
}

// CompileReadInstructionAddress implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileReadInstructionAddress(
	destinationRegister asm.Register,
	beforeAcquisitionTargetInstruction asm.Instruction,
) {
	ta.goasm.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
	ta.a.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
}

// CompileRegisterToRegisterWithArg implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToRegisterWithArg(
	instruction asm.Instruction,
	from, to asm.Register,
	arg byte,
) {
	ta.goasm.CompileRegisterToRegisterWithArg(instruction, from, to, arg)
	ta.a.CompileRegisterToRegisterWithArg(instruction, from, to, arg)
}

// CompileMemoryWithIndexToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileMemoryWithIndexToRegister(
	instruction asm.Instruction,
	srcBaseReg asm.Register,
	srcOffsetConst int64,
	srcIndex asm.Register,
	srcScale int16,
	dstReg asm.Register,
) {
	ta.goasm.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
	ta.a.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
}

// CompileMemoryWithIndexAndArgToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileMemoryWithIndexAndArgToRegister(
	instruction asm.Instruction,
	srcBaseReg asm.Register,
	srcOffsetConst int64,
	srcIndex asm.Register,
	srcScale int16,
	dstReg asm.Register,
	arg byte,
) {
	ta.goasm.CompileMemoryWithIndexAndArgToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg, arg)
	ta.a.CompileMemoryWithIndexAndArgToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg, arg)
}

// CompileRegisterToMemoryWithIndex implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToMemoryWithIndex(
	instruction asm.Instruction,
	srcReg, dstBaseReg asm.Register,
	dstOffsetConst int64,
	dstIndex asm.Register,
	dstScale int16,
) {
	ta.goasm.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
	ta.a.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
}

// CompileRegisterToMemoryWithIndexAndArg implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToMemoryWithIndexAndArg(
	instruction asm.Instruction,
	srcReg, dstBaseReg asm.Register,
	dstOffsetConst int64,
	dstIndex asm.Register,
	dstScale int16,
	arg byte,
) {
	ta.goasm.CompileRegisterToMemoryWithIndexAndArg(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale, arg)
	ta.a.CompileRegisterToMemoryWithIndexAndArg(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale, arg)
}

// CompileRegisterToConst implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToConst(
	instruction asm.Instruction,
	srcRegister asm.Register,
	value int64,
) asm.Node {
	ret := ta.goasm.CompileRegisterToConst(instruction, srcRegister, value)
	ret2 := ta.a.CompileRegisterToConst(instruction, srcRegister, value)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileRegisterToNone implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileRegisterToNone(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileRegisterToNone(instruction, register)
	ta.a.CompileRegisterToNone(instruction, register)
}

// CompileNoneToRegister implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileNoneToRegister(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileNoneToRegister(instruction, register)
	ta.a.CompileNoneToRegister(instruction, register)
}

// CompileNoneToMemory implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset int64) {
	ta.goasm.CompileNoneToMemory(instruction, baseReg, offset)
	ta.a.CompileNoneToMemory(instruction, baseReg, offset)
}

// CompileConstToMemory implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileConstToMemory(
	instruction asm.Instruction,
	value int64,
	dstbaseReg asm.Register,
	dstOffset int64,
) asm.Node {
	ret := ta.goasm.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	ret2 := ta.a.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileMemoryToConst implements the same method as documented on asm_amd64.Assembler.
func (ta *testAssembler) CompileMemoryToConst(
	instruction asm.Instruction,
	srcBaseReg asm.Register,
	srcOffset, value int64,
) asm.Node {
	ret := ta.goasm.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	ret2 := ta.a.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileStaticConstToRegister implements Assembler.CompileStaticConstToRegister.
func (ta *testAssembler) CompileStaticConstToRegister(asm.Instruction, *asm.StaticConst, asm.Register) (err error) {
	panic("CompileStaticConstToRegister cannot be supported by golang-asm")
}

// CompileRegisterToStaticConst implements Assembler.CompileRegisterToStaticConst.
func (ta *testAssembler) CompileRegisterToStaticConst(asm.Instruction, asm.Register, *asm.StaticConst) (err error) {
	panic("CompileRegisterToStaticConst cannot be supported by golang-asm")
}
