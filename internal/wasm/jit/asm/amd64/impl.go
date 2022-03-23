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

	offsetInBinary asm.NodeOffsetInBinary
	// jumpTarget holds the target node in the linked for the jump-kind instruction.
	jumpTarget *nodeImpl
	// prev holds the previous node to this node in the assembled linked list.
	prev *nodeImpl
	// next holds the next node from this node in the assembled linked list.
	next *nodeImpl

	types                    operandTypes
	srcReg, dstReg           asm.Register
	srcConst, dstConst       asm.ConstantValue
	srcMemIndex, dstMemIndex asm.Register
	srcMemScale, dstMemScale byte

	mode byte
}

// AssignJumpTarget implements asm.Node.AssignJumpTarget.
func (n *nodeImpl) AssignJumpTarget(target asm.Node) {
	n.jumpTarget = target.(*nodeImpl)
}

// AssignSourceConstant implements asm.Node.AssignSourceConstant.
func (n *nodeImpl) AssignDestinationConstant(value asm.ConstantValue) {
	n.dstConst = value
}

// AssignSourceConstant implements asm.Node.AssignSourceConstant.
func (n *nodeImpl) AssignSourceConstant(value asm.ConstantValue) {
	n.srcConst = value
}

// OffsetInBinary implements asm.Node.OffsetInBinary.
func (n *nodeImpl) OffsetInBinary() asm.NodeOffsetInBinary {
	return n.offsetInBinary
}

// String implements fmt.Stringer.
//
// This is for debugging purpose, and the format is almost same as the AT&T assembly syntax,
// meaning that this should look like "INSTRUCTION ${from}, ${to}" where each operand
// might be embraced by '[]' to represent the memory location.
func (n *nodeImpl) String() (ret string) {
	instName := instructionName(n.instruction)
	switch n.types {
	case operandTypesNoneToNone:
		ret = instName
	case operandTypesNoneToRegister:
		ret = fmt.Sprintf("%s %s", instName, registerName(n.dstReg))
	case operandTypesNoneToMemory:
		if n.dstMemIndex != asm.NilRegister {
			ret = fmt.Sprintf("%s [%s + 0x%x + %s*0x%x]", instName,
				registerName(n.dstReg), n.dstConst, registerName(n.dstMemIndex), n.dstMemScale)
		} else {
			ret = fmt.Sprintf("%s [%s + 0x%x]", instName, registerName(n.dstReg), n.dstConst)
		}
	case operandTypesNoneToBranch:
		ret = fmt.Sprintf("%s {%v}", instName, n.jumpTarget)
	case operandTypesRegisterToNone:
		ret = fmt.Sprintf("%s %s", instName, registerName(n.srcReg))
	case operandTypesRegisterToRegister:
		ret = fmt.Sprintf("%s %s, %s", instName, registerName(n.srcReg), registerName(n.dstReg))
	case operandTypesRegisterToMemory:
		if n.dstMemIndex != asm.NilRegister {
			ret = fmt.Sprintf("%s %s, [%s + 0x%x + %s*0x%x]", instName, registerName(n.srcReg),
				registerName(n.dstReg), n.dstConst, registerName(n.dstMemIndex), n.dstMemScale)
		} else {
			ret = fmt.Sprintf("%s %s, [%s + 0x%x]", instName, registerName(n.srcReg), registerName(n.dstReg), n.dstConst)
		}
	case operandTypesRegisterToConst:
		ret = fmt.Sprintf("%s %s, 0x%x", instName, registerName(n.srcReg), n.dstConst)
	case operandTypesMemoryToRegister:
		if n.srcMemIndex != asm.NilRegister {
			ret = fmt.Sprintf("%s [%s + %d + %s*0x%x], %s", instName,
				registerName(n.srcReg), n.srcConst, registerName(n.srcMemIndex), n.srcMemScale, registerName(n.dstReg))
		} else {
			ret = fmt.Sprintf("%s [%s + 0x%x], %s", instName, registerName(n.srcReg), n.srcConst, registerName(n.dstReg))
		}
	case operandTypesMemoryToConst:
		if n.srcMemIndex != asm.NilRegister {
			ret = fmt.Sprintf("%s [%s + %d + %s*0x%x], 0x%x", instName,
				registerName(n.srcReg), n.srcConst, registerName(n.srcMemIndex), n.srcMemScale, n.dstConst)
		} else {
			ret = fmt.Sprintf("%s [%s + 0x%x], 0x%x", instName, registerName(n.srcReg), n.srcConst, n.dstConst)
		}
	case operandTypesConstToMemory:
		if n.dstMemIndex != asm.NilRegister {
			ret = fmt.Sprintf("%s 0x%x, [%s + 0x%x + %s*0x%x]", instName, n.srcConst,
				registerName(n.dstReg), n.dstConst, registerName(n.dstMemIndex), n.dstMemScale)
		} else {
			ret = fmt.Sprintf("%s 0x%x, [%s + 0x%x]", instName, n.srcConst, registerName(n.dstReg), n.dstConst)
		}
	case operandTypesConstToRegister:
		ret = fmt.Sprintf("%s 0x%x, %s", instName, n.srcConst, registerName(n.dstReg))
	}
	return
}

// operandType represents where an operand is placed for an instruction.
// Note: this is almost the same as obj.AddrType in GO assembler.
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

// operandTypes represents the only combinations of two operandTypes used by wazero
type operandTypes struct{ src, dst operandType }

var (
	operandTypesNoneToNone         = operandTypes{operandTypeNone, operandTypeNone}
	operandTypesNoneToRegister     = operandTypes{operandTypeNone, operandTypeRegister}
	operandTypesNoneToMemory       = operandTypes{operandTypeNone, operandTypeMemory}
	operandTypesNoneToBranch       = operandTypes{operandTypeNone, operandTypeBranch}
	operandTypesRegisterToNone     = operandTypes{operandTypeRegister, operandTypeNone}
	operandTypesRegisterToRegister = operandTypes{operandTypeRegister, operandTypeRegister}
	operandTypesRegisterToMemory   = operandTypes{operandTypeRegister, operandTypeMemory}
	operandTypesRegisterToConst    = operandTypes{operandTypeRegister, operandTypeConst}
	operandTypesMemoryToRegister   = operandTypes{operandTypeMemory, operandTypeRegister}
	operandTypesMemoryToConst      = operandTypes{operandTypeMemory, operandTypeConst}
	operandTypesConstToRegister    = operandTypes{operandTypeConst, operandTypeRegister}
	operandTypesConstToMemory      = operandTypes{operandTypeConst, operandTypeMemory}
)

// String implements fmt.Stringer
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
func (a *assemblerImpl) newNode(instruction asm.Instruction, types operandTypes) *nodeImpl {
	n := &nodeImpl{
		instruction: instruction,
		prev:        nil,
		next:        nil,
		types:       types,
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

// encodeNode encodes the given node into writer.
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
	return a.newNode(instruction, operandTypesNoneToNone)
}

// CompileConstToRegister implements asm.AssemblerBase.CompileConstToRegister
func (a *assemblerImpl) CompileConstToRegister(instruction asm.Instruction, value asm.ConstantValue, destinationReg asm.Register) (inst asm.Node) {
	n := a.newNode(instruction, operandTypesConstToRegister)
	n.srcConst = value
	n.dstReg = destinationReg
	return n
}

// CompileRegisterToRegister implements asm.AssemblerBase.CompileRegisterToRegister
func (a *assemblerImpl) CompileRegisterToRegister(instruction asm.Instruction, from, to asm.Register) {
	n := a.newNode(instruction, operandTypesRegisterToRegister)
	n.srcReg = from
	n.dstReg = to
}

// CompileMemoryToRegister implements asm.AssemblerBase.CompileMemoryToRegister
func (a *assemblerImpl) CompileMemoryToRegister(instruction asm.Instruction, sourceBaseReg asm.Register, sourceOffsetConst asm.ConstantValue, destinationReg asm.Register) {
	n := a.newNode(instruction, operandTypesMemoryToRegister)
	n.srcReg = sourceBaseReg
	n.srcConst = sourceOffsetConst
	n.dstReg = destinationReg
}

// CompileRegisterToMemory implements asm.AssemblerBase.CompileRegisterToMemory
func (a *assemblerImpl) CompileRegisterToMemory(instruction asm.Instruction, sourceRegister asm.Register, destinationBaseRegister asm.Register, destinationOffsetConst asm.ConstantValue) {
	n := a.newNode(instruction, operandTypesRegisterToMemory)
	n.srcReg = sourceRegister
	n.dstReg = destinationBaseRegister
	n.dstConst = destinationOffsetConst
}

// CompileJump implements asm.AssemblerBase.CompileJump
func (a *assemblerImpl) CompileJump(jmpInstruction asm.Instruction) asm.Node {
	return a.newNode(jmpInstruction, operandTypesNoneToBranch)
}

// CompileJumpToMemory implements asm.AssemblerBase.CompileJumpToMemory
func (a *assemblerImpl) CompileJumpToMemory(jmpInstruction asm.Instruction, baseReg asm.Register, offset asm.ConstantValue) {
	n := a.newNode(jmpInstruction, operandTypesNoneToMemory)
	n.dstReg = baseReg
	n.dstConst = offset
}

// CompileJumpToRegister implements asm.AssemblerBase.CompileJumpToRegister
func (a *assemblerImpl) CompileJumpToRegister(jmpInstruction asm.Instruction, reg asm.Register) {
	n := a.newNode(jmpInstruction, operandTypesNoneToRegister)
	n.dstReg = reg
}

// CompileReadInstructionAddress implements asm.AssemblerBase.CompileReadInstructionAddress
func (a *assemblerImpl) CompileReadInstructionAddress(destinationRegister asm.Register, beforeAcquisitionTargetInstruction asm.Instruction) {
	// TODO
}

// CompileRegisterToRegisterWithMode implements assembler.CompileRegisterToRegisterWithMode
func (a *assemblerImpl) CompileRegisterToRegisterWithMode(instruction asm.Instruction, from, to asm.Register, mode Mode) {
	n := a.newNode(instruction, operandTypesRegisterToRegister)
	n.srcReg = from
	n.dstReg = to
	n.mode = mode
}

// CompileMemoryWithIndexToRegister implements assembler.CompileMemoryWithIndexToRegister
func (a *assemblerImpl) CompileMemoryWithIndexToRegister(instruction asm.Instruction, srcBaseReg asm.Register, srcOffsetConst asm.ConstantValue, srcIndex asm.Register, srcScale int16, dstReg asm.Register) {
	n := a.newNode(instruction, operandTypesMemoryToRegister)
	n.srcReg = srcBaseReg
	n.srcConst = srcOffsetConst
	n.srcMemIndex = srcIndex
	n.srcMemScale = byte(srcScale)
	n.dstReg = dstReg
}

// CompileRegisterToMemoryWithIndex implements assembler.CompileRegisterToMemoryWithIndex
func (a *assemblerImpl) CompileRegisterToMemoryWithIndex(instruction asm.Instruction, srcReg asm.Register, dstBaseReg asm.Register, dstOffsetConst asm.ConstantValue, dstIndex asm.Register, dstScale int16) {
	n := a.newNode(instruction, operandTypesRegisterToMemory)
	n.srcReg = srcReg
	n.dstReg = dstBaseReg
	n.dstConst = dstOffsetConst
	n.dstMemIndex = dstIndex
	n.dstMemScale = byte(dstScale)
}

// CompileRegisterToConst implements assembler.CompileRegisterToConst
func (a *assemblerImpl) CompileRegisterToConst(instruction asm.Instruction, srcRegister asm.Register, value asm.ConstantValue) asm.Node {
	n := a.newNode(instruction, operandTypesRegisterToConst)
	n.srcReg = srcRegister
	n.dstConst = value
	return n
}

// CompileRegisterToNone implements assembler.CompileRegisterToNone
func (a *assemblerImpl) CompileRegisterToNone(instruction asm.Instruction, register asm.Register) {
	n := a.newNode(instruction, operandTypesRegisterToNone)
	n.srcReg = register
}

// CompileNoneToRegister implements assembler.CompileNoneToRegister
func (a *assemblerImpl) CompileNoneToRegister(instruction asm.Instruction, register asm.Register) {
	n := a.newNode(instruction, operandTypesNoneToRegister)
	n.dstReg = register
}

// CompileNoneToMemory implements assembler.CompileNoneToMemory
func (a *assemblerImpl) CompileNoneToMemory(instruction asm.Instruction, baseReg asm.Register, offset asm.ConstantValue) {
	n := a.newNode(instruction, operandTypesNoneToMemory)
	n.dstReg = baseReg
	n.dstConst = offset
}

// CompileConstToMemory implements assembler.CompileConstToMemory
func (a *assemblerImpl) CompileConstToMemory(instruction asm.Instruction, value asm.ConstantValue, dstbaseReg asm.Register, dstOffset asm.ConstantValue) asm.Node {
	n := a.newNode(instruction, operandTypesConstToMemory)
	n.srcConst = value
	n.dstReg = dstbaseReg
	n.dstConst = dstOffset
	return n
}

// CompileMemoryToConst implements assembler.CompileMemoryToConst
func (a *assemblerImpl) CompileMemoryToConst(instruction asm.Instruction, srcBaseReg asm.Register, srcOffset asm.ConstantValue, value asm.ConstantValue) asm.Node {
	n := a.newNode(instruction, operandTypesMemoryToConst)
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
