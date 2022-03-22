package amd64

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/internal/wasm/jit/asm"
)

// nodeImpl implements asm.Node for amd64.
type nodeImpl struct {
	instruction asm.Instruction

	// offsetInBinary represents the offset of this node in the final binary.
	offsetInBinary int64
	// jumpTarget holds the target node in the linked for the jump-kind instruction.
	jumpTarget *nodeImpl
	// prev holds the previous node to this node in the assembled linked list.
	prev *nodeImpl
	// next holds the next node from this node in the assembled linked list.
	next *nodeImpl

	types                    operandTypes
	srcReg, dstReg           asm.Register
	srcConst, dstConst       int64
	srcMemIndex, dstMemIndex asm.Register
	srcMemScale, dstMemScale byte

	mode byte
}

// AssignJumpTarget implements asm.Node.AssignJumpTarget.
func (n *nodeImpl) AssignJumpTarget(target asm.Node) {
	n.jumpTarget = target.(*nodeImpl)
}

// AssignSourceConstant implements asm.Node.AssignSourceConstant.
func (n *nodeImpl) AssignDestinationConstant(value int64) {
	n.dstConst = value
}

// AssignSourceConstant implements asm.Node.AssignSourceConstant.
func (n *nodeImpl) AssignSourceConstant(value int64) {
	n.srcConst = value
}

// OffsetInBinary implements asm.Node.OffsetInBinary.
func (n *nodeImpl) OffsetInBinary() int64 {
	return n.offsetInBinary
}

// String implements fmt.Stringer.
func (n *nodeImpl) String() string {
	return "TODO"
}

type operandType byte

const (
	operandTypeNone operandType = iota
	operandTypeRegister
	operandTypeMemory
	operandTypeConst
	operandTypeBranch
)

func (o operandType) String() (ret string) {
	switch o {
	case operandTypeNone:
		ret = "none"
	case operandTypeRegister:
		ret = "register"
	case operandTypeMemory:
		ret = "memory"
	case operandTypeConst:
		ret = "const"
	case operandTypeBranch:
		ret = "branch"
	}
	return
}

type operandTypes struct{ src, dst operandType }

var (
	operandTypesNoneToNone         = operandTypes{operandTypeNone, operandTypeNone}
	operandTypesNoneToRegister     = operandTypes{operandTypeNone, operandTypeRegister}
	operandTypesNoneToMemory       = operandTypes{operandTypeNone, operandTypeMemory}
	operandTypesNoneToBranch       = operandTypes{operandTypeNone, operandTypeBranch}
	operandTypesRegisterToNone     = operandTypes{operandTypeRegister, operandTypeNone}
	operandTypesRegisterToRegister = operandTypes{operandTypeRegister, operandTypeRegister}
	operandTypesRegisterToMemory   = operandTypes{operandTypeRegister, operandTypeRegister}
	operandTypesRegisterToConst    = operandTypes{operandTypeRegister, operandTypeConst}
	operandTypesMemoryToRegister   = operandTypes{operandTypeMemory, operandTypeRegister}
)

func (o operandTypes) String() string {
	return fmt.Sprintf("from:%s,to:%s", o.src, o.dst)
}

// assemblerImpl implements Assembler.
type assemblerImpl struct {
	asm.BaseAssemblerImpl

	root, current *nodeImpl
}

var _ Assembler = &assemblerImpl{}

// newNode creates a new Node and appends it into the linked list.
func (a *assemblerImpl) newNode(instruction asm.Instruction, srcType, dstType operandType) *nodeImpl {
	n := &nodeImpl{
		instruction: instruction,
		prev:        nil,
		next:        nil,
		types:       operandTypes{src: srcType, dst: dstType},
	}
	a.addNode(n)
	return n
}

// addNode appends the new node into the linked list.
func (a *assemblerImpl) addNode(node *nodeImpl) {
	if a.root == nil {
		a.root = node
		a.current = node
	} else {
		parent := a.current
		parent.next = node
		a.current = node
	}
}

func (a *assemblerImpl) encodeNode(w io.Writer, n *nodeImpl) (err error) {
	switch n.types {
	case operandTypesNoneToNone:
		err = a.encodeNoneToNone(w, n)
	case operandTypesNoneToRegister:
		err = a.encodeNoneToRegister(w, n)
	case operandTypesNoneToMemory:
		err = a.encodeNoneToMemory(w, n)
	case operandTypesNoneToBranch:
		err = a.encodeNoneToBranch(w, n)
	case operandTypesRegisterToNone:
		err = a.encodeRegisterToNone(w, n)
	case operandTypesRegisterToRegister:
		err = a.encodeRegisterToRegister(w, n)
	case operandTypesRegisterToMemory:
		err = a.encodeRegisterToMemory(w, n)
	case operandTypesRegisterToConst:
		err = a.encodeRegisterToConst(w, n)
	case operandTypesMemoryToRegister:
		err = a.encodeMemoryToRegister(w, n)
	default:
		err = fmt.Errorf("encoder undefined for [%s] operand type", n.types)
	}
	return
}

// Assemble implements asm.AssemblerBase
func (a *assemblerImpl) Assemble() ([]byte, error) {
	w := bytes.NewBuffer(nil)
	// TODO: NOP padding for jumps before encodes: https://github.com/golang/go/issues/35881

	for n := a.root; n != nil; n = n.next {
		if err := a.encodeNode(w, n); err != nil {
			return nil, err
		}
	}

	code := w.Bytes()
	for _, cb := range a.OnGenerateCallbacks {
		if err := cb(code); err != nil {
			return nil, err
		}
	}
	return code, nil
}

// CompileReadInstructionAddress implements asm.AssemblerBase.CompileReadInstructionAddress
func (a *assemblerImpl) CompileStandAlone(instruction asm.Instruction) asm.Node {
	return a.newNode(instruction, operandTypeNone, operandTypeNone)
}

// CompileConstToRegister implements asm.AssemblerBase.CompileConstToRegister
func (a *assemblerImpl) CompileConstToRegister(instruction asm.Instruction, value int64, destinationReg asm.Register) (inst asm.Node) {
	n := a.newNode(instruction, operandTypeConst, operandTypeRegister)
	n.srcConst = value
	n.dstReg = destinationReg
	return n
}

// CompileRegisterToRegister implements asm.AssemblerBase.CompileRegisterToRegister
func (a *assemblerImpl) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	n := a.newNode(instruction, operandTypeRegister, operandTypeRegister)
	n.srcReg = from
	n.dstReg = to
}

// CompileMemoryToRegister implements asm.AssemblerBase.CompileMemoryToRegister
func (a *assemblerImpl) CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst int64, destinationReg asm.Register) {
	n := a.newNode(instruction, operandTypeMemory, operandTypeRegister)
	n.srcReg = sourceBaseReg
	n.srcConst = sourceOffsetConst
	n.dstReg = destinationReg
}

// CompileRegisterToMemory implements asm.AssemblerBase.CompileRegisterToMemory
func (a *assemblerImpl) CompileRegisterToMemory(instruction asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst int64) {
	n := a.newNode(instruction, operandTypeRegister, operandTypeMemory)
	n.srcReg = sourceRegister
	n.dstReg = destinationBaseRegister
	n.dstConst = destinationOffsetConst
}

// CompileJump implements asm.AssemblerBase.CompileJump
func (a *assemblerImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	return a.newNode(jmpInstruction, operandTypeNone, operandTypeBranch)
}

// CompileJumpToMemory implements asm.AssemblerBase.CompileJumpToMemory
func (a *assemblerImpl) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register, offset int64) {
	n := a.newNode(jmpInstruction, operandTypeNone, operandTypeMemory)
	n.dstReg = baseReg
	n.dstConst = offset
}

// CompileJumpToRegister implements asm.AssemblerBase.CompileJumpToRegister
func (a *assemblerImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	n := a.newNode(jmpInstruction, operandTypeNone, operandTypeRegister)
	n.dstReg = reg
}

// CompileReadInstructionAddress implements asm.AssemblerBase.CompileReadInstructionAddress
func (a *assemblerImpl) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	// TODO
}

// CompileModeRegisterToRegister implements assembler.CompileModeRegisterToRegister
func (a *assemblerImpl) CompileModeRegisterToRegister(instruction asm.Instruction, from, to asm.Register, mode int64) {
	n := a.newNode(instruction, operandTypeRegister, operandTypeRegister)
	n.srcReg = from
	n.dstReg = to
	n.mode = byte(mode)
}

// CompileMemoryWithIndexToRegister implements assembler.CompileMemoryWithIndexToRegister
func (a *assemblerImpl) CompileMemoryWithIndexToRegister(instruction asm.Instruction, srcBaseReg asm.Register, srcOffsetConst int64, srcIndex asm.Register, srcScale int16, dstReg asm.Register) {
	n := a.newNode(instruction, operandTypeMemory, operandTypeRegister)
	n.srcReg = srcBaseReg
	n.srcConst = srcOffsetConst
	n.srcMemIndex = srcIndex
	n.srcMemScale = byte(srcScale)
	n.dstReg = dstReg
}

// CompileRegisterToMemoryWithIndex implements assembler.CompileRegisterToMemoryWithIndex
func (a *assemblerImpl) CompileRegisterToMemoryWithIndex(instruction asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst int64, dstIndex asm.Register, dstScale int16) {
	n := a.newNode(instruction, operandTypeRegister, operandTypeMemory)
	n.srcReg = srcReg
	n.dstReg = dstBaseReg
	n.dstConst = dstOffsetConst
	n.dstMemIndex = dstIndex
	n.dstMemScale = byte(dstScale)
}

// CompileRegisterToConst implements assembler.CompileRegisterToConst
func (a *assemblerImpl) CompileRegisterToConst(instruction asm.Instruction, srcRegister asm.Register, value int64) asm.Node {
	n := a.newNode(instruction, operandTypeRegister, operandTypeConst)
	n.srcReg = srcRegister
	n.dstConst = value
	return n
}

// CompileRegisterToNone implements assembler.CompileRegisterToNone
func (a *assemblerImpl) CompileRegisterToNone(instruction asm.Instruction, register asm.Register) {
	n := a.newNode(instruction, operandTypeRegister, operandTypeNone)
	n.srcReg = register
}

// CompileNoneToRegister implements assembler.CompileNoneToRegister
func (a *assemblerImpl) CompileNoneToRegister(instruction asm.Instruction, register asm.Register) {
	n := a.newNode(instruction, operandTypeNone, operandTypeRegister)
	n.dstReg = register
}

// CompileNoneToMemory implements assembler.CompileNoneToMemory
func (a *assemblerImpl) CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset int64) {
	n := a.newNode(instruction, operandTypeNone, operandTypeMemory)
	n.dstReg = baseReg
	n.dstConst = offset
}

func (a *assemblerImpl) CompileConstToMemory(instruction asm.Instruction, value int64, dstbaseReg asm.Register, dstOffset int64) asm.Node {
	n := a.newNode(instruction, operandTypeConst, operandTypeMemory)
	n.srcConst = value
	n.dstReg = dstbaseReg
	n.dstConst = dstOffset
	return n
}

func (a *assemblerImpl) CompileMemoryToConst(instruction asm.Instruction, srcBaseReg asm.Register, srcOffset int64, value int64) asm.Node {
	n := a.newNode(instruction, operandTypeMemory, operandTypeConst)
	n.srcReg = srcBaseReg
	n.srcConst = srcOffset
	n.dstConst = value
	return n
}

func (a *assemblerImpl) encodeNoneToNone(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeNoneToRegister(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeNoneToMemory(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeNoneToBranch(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeRegisterToNone(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeRegisterToRegister(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeRegisterToMemory(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeRegisterToConst(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}

func (a *assemblerImpl) encodeMemoryToRegister(w io.Writer, n *nodeImpl) error {
	return errors.New("TODO")
}
