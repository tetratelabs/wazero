package amd64_debug

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/objabi"

	"github.com/heeus/inv-wazero/internal/asm"
	asm_amd64 "github.com/heeus/inv-wazero/internal/asm/amd64"
	"github.com/heeus/inv-wazero/internal/asm/golang_asm"
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

// testAssembler implements Assembler.
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

// AssignJumpTarget implements asm.Node.AssignJumpTarget.
func (tn *testNode) AssignJumpTarget(target asm.Node) {
	targetTestNode := target.(*testNode)
	tn.goasm.AssignJumpTarget(targetTestNode.goasm)
	tn.n.AssignJumpTarget(targetTestNode.n)
}

// AssignDestinationConstant implements asm.Node.AssignDestinationConstant.
func (tn *testNode) AssignDestinationConstant(value asm.ConstantValue) {
	tn.goasm.AssignDestinationConstant(value)
	tn.n.AssignDestinationConstant(value)
}

// AssignSourceConstant implements asm.Node.AssignSourceConstant.
func (tn *testNode) AssignSourceConstant(value asm.ConstantValue) {
	tn.goasm.AssignSourceConstant(value)
	tn.n.AssignSourceConstant(value)
}

// OffsetInBinary implements asm.Node.OffsetInBinary.
func (tn *testNode) OffsetInBinary() asm.NodeOffsetInBinary {
	return tn.goasm.OffsetInBinary()
}

// Assemble implements Assembler.Assemble.
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

// SetJumpTargetOnNext implements Assembler.SetJumpTargetOnNext.
func (ta *testAssembler) SetJumpTargetOnNext(nodes ...asm.Node) {
	for _, n := range nodes {
		targetTestNode := n.(*testNode)
		ta.goasm.SetJumpTargetOnNext(targetTestNode.goasm)
		ta.a.SetJumpTargetOnNext(targetTestNode.n)
	}
}

// BuildJumpTable implements Assembler.BuildJumpTable.
func (ta *testAssembler) BuildJumpTable(table []byte, initialInstructions []asm.Node) {
	ta.goasm.BuildJumpTable(table, initialInstructions)
	ta.a.BuildJumpTable(table, initialInstructions)
}

// CompileStandAlone implements Assembler.CompileStandAlone.
func (ta *testAssembler) CompileStandAlone(instruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileStandAlone(instruction)
	ret2 := ta.a.CompileStandAlone(instruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileConstToRegister implements Assembler.CompileConstToRegister.
func (ta *testAssembler) CompileConstToRegister(instruction asm.Instruction, value asm.ConstantValue, destinationReg asm.Register) asm.Node {
	ret := ta.goasm.CompileConstToRegister(instruction, value, destinationReg)
	ret2 := ta.a.CompileConstToRegister(instruction, value, destinationReg)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileRegisterToRegister implements Assembler.CompileRegisterToRegister.
func (ta *testAssembler) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	ta.goasm.CompileRegisterToRegister(instruction, from, to)
	ta.a.CompileRegisterToRegister(instruction, from, to)
}

// CompileMemoryToRegister implements Assembler.CompileMemoryToRegister.
func (ta *testAssembler) CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst asm.ConstantValue, destinationReg asm.Register) {
	ta.goasm.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
	ta.a.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
}

// CompileRegisterToMemory implements Assembler.CompileRegisterToMemory.
func (ta *testAssembler) CompileRegisterToMemory(instruction asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst asm.ConstantValue) {
	ta.goasm.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
	ta.a.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
}

// CompileJump implements Assembler.CompileJump.
func (ta *testAssembler) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileJump(jmpInstruction)
	ret2 := ta.a.CompileJump(jmpInstruction)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileJumpToMemory implements Assembler.CompileJumpToMemory.
func (ta *testAssembler) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register, offset asm.ConstantValue) {
	ta.goasm.CompileJumpToMemory(jmpInstruction, baseReg, offset)
	ta.a.CompileJumpToMemory(jmpInstruction, baseReg, offset)
}

// CompileJumpToRegister implements Assembler.CompileJumpToRegister.
func (ta *testAssembler) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	ta.goasm.CompileJumpToRegister(jmpInstruction, reg)
	ta.a.CompileJumpToRegister(jmpInstruction, reg)
}

// CompileReadInstructionAddress implements Assembler.CompileReadInstructionAddress.
func (ta *testAssembler) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	ta.goasm.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
	ta.a.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
}

// CompileRegisterToRegisterWithMode implements Assembler.CompileRegisterToRegisterWithMode.
func (ta *testAssembler) CompileRegisterToRegisterWithMode(instruction asm.Instruction, from, to asm.Register, mode asm_amd64.Mode) {
	ta.goasm.CompileRegisterToRegisterWithMode(instruction, from, to, mode)
	ta.a.CompileRegisterToRegisterWithMode(instruction, from, to, mode)
}

// CompileMemoryWithIndexToRegister implements Assembler.CompileMemoryWithIndexToRegister.
func (ta *testAssembler) CompileMemoryWithIndexToRegister(instruction asm.Instruction, srcBaseReg asm.Register, srcOffsetConst int64, srcIndex asm.Register, srcScale int16, dstReg asm.Register) {
	ta.goasm.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
	ta.a.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
}

// CompileRegisterToMemoryWithIndex implements Assembler.CompileRegisterToMemoryWithIndex.
func (ta *testAssembler) CompileRegisterToMemoryWithIndex(instruction asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale int16) {
	ta.goasm.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
	ta.a.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
}

// CompileRegisterToConst implements Assembler.CompileRegisterToConst.
func (ta *testAssembler) CompileRegisterToConst(instruction asm.Instruction, srcRegister asm.Register, value int64) asm.Node {
	ret := ta.goasm.CompileRegisterToConst(instruction, srcRegister, value)
	ret2 := ta.a.CompileRegisterToConst(instruction, srcRegister, value)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileRegisterToNone implements Assembler.CompileRegisterToNone.
func (ta *testAssembler) CompileRegisterToNone(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileRegisterToNone(instruction, register)
	ta.a.CompileRegisterToNone(instruction, register)
}

// CompileNoneToRegister implements Assembler.CompileNoneToRegister.
func (ta *testAssembler) CompileNoneToRegister(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileNoneToRegister(instruction, register)
	ta.a.CompileNoneToRegister(instruction, register)
}

// CompileNoneToMemory implements Assembler.CompileNoneToMemory.
func (ta *testAssembler) CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset int64) {
	ta.goasm.CompileNoneToMemory(instruction, baseReg, offset)
	ta.a.CompileNoneToMemory(instruction, baseReg, offset)
}

// CompileConstToMemory implements Assembler.CompileConstToMemory.
func (ta *testAssembler) CompileConstToMemory(instruction asm.Instruction, value int64, dstbaseReg asm.Register, dstOffset int64) asm.Node {
	ret := ta.goasm.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	ret2 := ta.a.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}

// CompileMemoryToConst implements Assembler.CompileMemoryToConst.
func (ta *testAssembler) CompileMemoryToConst(instruction asm.Instruction, srcBaseReg asm.Register, srcOffset int64, value int64) asm.Node {
	ret := ta.goasm.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	ret2 := ta.a.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	return &testNode{goasm: ret.(*golang_asm.GolangAsmNode), n: ret2.(*asm_amd64.NodeImpl)}
}
