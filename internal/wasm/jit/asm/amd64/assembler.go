package amd64

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/twitchyliquid64/golang-asm/objabi"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

var NewAssembler func() (Assembler, error) = NewAssemblerImpl

func NewAssemblerImpl() (Assembler, error) {
	a := newAssemblerImpl()
	return a, nil
}

// NewAssemblerForTesting can be used for ensuring that our assembler produces exactly the same binary as Go.
// Disabled by default, but assigning this to NewAssembler allows us to debug assembler's bug.
func NewAssemblerForTesting() (Assembler, error) {
	goasm, _ := newGolangAsmAssembler()
	a := newAssemblerImpl()

	// If nop padding is enabled, it is really difficult to match the logics of golang-asm since it's so complex
	// and not well-documented. Given that NOP padding is just padding NOPs literally, and it doesn't affect
	// the semantics of program, we should be fine to debug without padding enabled.
	objabi.GOAMD64 = "disable"
	a.enablePadding = false
	return &testAssembler{a: a, goasm: goasm}, nil
}

type Assembler interface {
	asm.AssemblerBase
	// CompileRegisterToRegisterWithMode adds an instruction where source and destination
	// are `from` and `to` registers and the instruction's "mode" is specified by `mode`.
	CompileRegisterToRegisterWithMode(instruction asm.Instruction, from, to asm.Register, mode Mode)
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

// Mode represents a mode for specific instruction.
// For example, ROUND** instructions' behavior can be modified "mode" constant.
// See https://www.felixcloutier.com/x86/roundss for ROUNDSS as an example.
type Mode = byte

// testAssembler implements Assembler.
// This assembler ensures that our assembler produces exactly the same binary as the Go's official assembler.
// Disabled by default, and can be used for debugging only.
type testAssembler struct {
	goasm *assemblerGoAsmImpl
	a     *assemblerImpl
}

// testNode implements asm.Node for the usage with testAssembler.
type testNode struct {
	n     *nodeImpl
	goasm *asm.GolangAsmNode
}

func (tn *testNode) String() string {
	return tn.n.String()
}

func (tn *testNode) AssignJumpTarget(target asm.Node) {
	targetTestNode := target.(*testNode)
	tn.goasm.AssignJumpTarget(targetTestNode.goasm)
	tn.n.AssignJumpTarget(targetTestNode.n)
}

func (tn *testNode) AssignDestinationConstant(value asm.ConstantValue) {
	tn.goasm.AssignDestinationConstant(value)
	tn.n.AssignDestinationConstant(value)
}

func (tn *testNode) AssignSourceConstant(value asm.ConstantValue) {
	tn.goasm.AssignSourceConstant(value)
	tn.n.AssignSourceConstant(value)
}

func (tn *testNode) OffsetInBinary() asm.NodeOffsetInBinary {
	return tn.goasm.OffsetInBinary()
}

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

func (ta *testAssembler) SetJumpTargetOnNext(nodes ...asm.Node) {
	for _, n := range nodes {
		targetTestNode := n.(*testNode)
		ta.goasm.SetJumpTargetOnNext(targetTestNode.goasm)
		ta.a.SetJumpTargetOnNext(targetTestNode.n)
	}
}

func (ta *testAssembler) BuildJumpTable(table []byte, initialInstructions []asm.Node) {
	ta.goasm.BuildJumpTable(table, initialInstructions)
	ta.a.BuildJumpTable(table, initialInstructions)
}

func (ta *testAssembler) CompileStandAlone(instruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileStandAlone(instruction)
	ret2 := ta.a.CompileStandAlone(instruction)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}

func (ta *testAssembler) CompileConstToRegister(instruction asm.Instruction, value asm.ConstantValue, destinationReg asm.Register) asm.Node {
	ret := ta.goasm.CompileConstToRegister(instruction, value, destinationReg)
	ret2 := ta.a.CompileConstToRegister(instruction, value, destinationReg)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}

func (ta *testAssembler) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	ta.goasm.CompileRegisterToRegister(instruction, from, to)
	ta.a.CompileRegisterToRegister(instruction, from, to)

}

func (ta *testAssembler) CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst asm.ConstantValue, destinationReg asm.Register) {
	ta.goasm.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
	ta.a.CompileMemoryToRegister(instruction, sourceBaseReg, sourceOffsetConst, destinationReg)
}

func (ta *testAssembler) CompileRegisterToMemory(instruction asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst asm.ConstantValue) {
	ta.goasm.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
	ta.a.CompileRegisterToMemory(instruction, sourceRegister, destinationBaseRegister, destinationOffsetConst)
}

func (ta *testAssembler) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	ret := ta.goasm.CompileJump(jmpInstruction)
	ret2 := ta.a.CompileJump(jmpInstruction)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}

func (ta *testAssembler) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register, offset asm.ConstantValue) {
	ta.goasm.CompileJumpToMemory(jmpInstruction, baseReg, offset)
	ta.a.CompileJumpToMemory(jmpInstruction, baseReg, offset)
}

func (ta *testAssembler) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	ta.goasm.CompileJumpToRegister(jmpInstruction, reg)
	ta.a.CompileJumpToRegister(jmpInstruction, reg)
}

func (ta *testAssembler) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	ta.goasm.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
	ta.a.CompileReadInstructionAddress(destinationRegister, beforeAcquisitionTargetInstruction)
}

func (ta *testAssembler) CompileRegisterToRegisterWithMode(instruction asm.Instruction, from, to asm.Register, mode Mode) {
	ta.goasm.CompileRegisterToRegisterWithMode(instruction, from, to, mode)
	ta.a.CompileRegisterToRegisterWithMode(instruction, from, to, mode)
}

func (ta *testAssembler) CompileMemoryWithIndexToRegister(instruction asm.Instruction, srcBaseReg asm.Register, srcOffsetConst int64, srcIndex asm.Register, srcScale int16, dstReg asm.Register) {
	ta.goasm.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
	ta.a.CompileMemoryWithIndexToRegister(instruction, srcBaseReg, srcOffsetConst, srcIndex, srcScale, dstReg)
}

func (ta *testAssembler) CompileRegisterToMemoryWithIndex(instruction asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale int16) {
	ta.goasm.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
	ta.a.CompileRegisterToMemoryWithIndex(instruction, srcReg, dstBaseReg, dstOffsetConst, dstIndex, dstScale)
}

func (ta *testAssembler) CompileRegisterToConst(instruction asm.Instruction, srcRegister asm.Register, value int64) asm.Node {
	ret := ta.goasm.CompileRegisterToConst(instruction, srcRegister, value)
	ret2 := ta.a.CompileRegisterToConst(instruction, srcRegister, value)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}

func (ta *testAssembler) CompileRegisterToNone(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileRegisterToNone(instruction, register)
	ta.a.CompileRegisterToNone(instruction, register)
}

func (ta *testAssembler) CompileNoneToRegister(instruction asm.Instruction, register asm.Register) {
	ta.goasm.CompileNoneToRegister(instruction, register)
	ta.a.CompileNoneToRegister(instruction, register)
}

func (ta *testAssembler) CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset int64) {
	ta.goasm.CompileNoneToMemory(instruction, baseReg, offset)
	ta.a.CompileNoneToMemory(instruction, baseReg, offset)
}

func (ta *testAssembler) CompileConstToMemory(instruction asm.Instruction, value int64, dstbaseReg asm.Register, dstOffset int64) asm.Node {
	ret := ta.goasm.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	ret2 := ta.a.CompileConstToMemory(instruction, value, dstbaseReg, dstOffset)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}

func (ta *testAssembler) CompileMemoryToConst(instruction asm.Instruction, srcBaseReg asm.Register, srcOffset int64, value int64) asm.Node {
	ret := ta.goasm.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	ret2 := ta.a.CompileMemoryToConst(instruction, srcBaseReg, srcOffset, value)
	return &testNode{goasm: ret.(*asm.GolangAsmNode), n: ret2.(*nodeImpl)}
}
