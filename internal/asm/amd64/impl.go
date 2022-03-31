package asm_amd64

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/internal/asm"
)

// nodeImpl implements asm.Node for amd64.
type nodeImpl struct {
	instruction asm.Instruction

	offsetInBinary asm.NodeOffsetInBinary
	// jumpTarget holds the target node in the linked for the jump-kind instruction.
	jumpTarget *nodeImpl
	flag       nodeFlag
	// next holds the next node from this node in the assembled linked list.
	next *nodeImpl

	types                    operandTypes
	srcReg, dstReg           asm.Register
	srcConst, dstConst       asm.ConstantValue
	srcMemIndex, dstMemIndex asm.Register
	srcMemScale, dstMemScale byte

	mode byte

	// readInstructionAddressBeforeTargetInstruction holds the instruction right before the target of
	// read instruction address instruction. See asm.assemblerBase.CompileReadInstructionAddress.
	readInstructionAddressBeforeTargetInstruction asm.Instruction

	// jumpOrigins hold all the nodes trying to jump into this node. In other workds, all the nodes with .jumpTarget == this.
	jumpOrigins map[*nodeImpl]struct{}
}

type nodeFlag byte

const (
	// nodeFlagInitializedForEncoding is always set to indicate that node is already initialized. Notably, this is used to judge
	// whether a jump is backward or forward before encoding.
	nodeFlagInitializedForEncoding nodeFlag = (1 << iota)
	nodeFlagBackwardJump
	// forceLongJump is set to false by default and only used by forward branch jumps, which means .jumpTarget != nil and
	// the target node is encoded afoter this node. False by default means that that we encode all the jumps with jumpTarget
	// as short jump (i.e. relative signed 8-bit integer offset jump) and try to encode as small as possible.
	nodeFlagShortForwardJump
)

func (n *nodeImpl) isInitializedForEncoding() bool {
	return n.flag&nodeFlagInitializedForEncoding != 0
}

func (n *nodeImpl) isJumpNode() bool {
	return n.jumpTarget != nil
}

func (n *nodeImpl) isBackwardJump() bool {
	return n.isJumpNode() && (n.flag&nodeFlagBackwardJump != 0)
}

func (n *nodeImpl) isForwardJump() bool {
	return n.isJumpNode() && (n.flag&nodeFlagBackwardJump == 0)
}

func (n *nodeImpl) isForwardShortJump() bool {
	return n.isForwardJump() && n.flag&nodeFlagShortForwardJump != 0
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
	enablePadding   bool
	root, current   *nodeImpl
	buf             *bytes.Buffer
	forceReAssemble bool
}

func newAssemblerImpl() *assemblerImpl {
	return &assemblerImpl{buf: bytes.NewBuffer(nil), enablePadding: true}
}

// newNode creates a new Node and appends it into the linked list.
func (a *assemblerImpl) newNode(instruction asm.Instruction, types operandTypes) *nodeImpl {
	n := &nodeImpl{
		instruction: instruction,
		next:        nil,
		types:       types,
		jumpOrigins: map[*nodeImpl]struct{}{},
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

	for _, o := range a.SetBranchTargetOnNextNodes {
		origin := o.(*nodeImpl)
		origin.jumpTarget = node
	}
	a.SetBranchTargetOnNextNodes = nil
}

// encodeNode encodes the given node into writer.
func (a *assemblerImpl) encodeNode(n *nodeImpl) (err error) {
	switch n.types {
	case operandTypesNoneToNone:
		err = a.encodeNoneToNone(n)
	case operandTypesNoneToRegister:
		err = a.encodeNoneToRegister(n)
	case operandTypesNoneToMemory:
		err = a.encodeNoneToMemory(n)
	case operandTypesNoneToBranch:
		// Branching operand can be encoded as relative jumps.
		err = a.encodeRelativeJump(n)
	case operandTypesRegisterToNone:
		err = a.encodeRegisterToNone(n)
	case operandTypesRegisterToRegister:
		err = a.encodeRegisterToRegister(n)
	case operandTypesRegisterToMemory:
		err = a.encodeRegisterToMemory(n)
	case operandTypesRegisterToConst:
		err = a.encodeRegisterToConst(n)
	case operandTypesMemoryToRegister:
		err = a.encodeMemoryToRegister(n)
	case operandTypesConstToRegister:
		err = a.encodeConstToRegister(n)
	case operandTypesConstToMemory:
		err = a.encodeConstToMemory(n)
	case operandTypesMemoryToConst:
		err = a.encodeMemoryToConst(n)
	default:
		err = fmt.Errorf("encoder undefined for [%s] operand type", n.types)
	}
	return
}

// Assemble implements asm.AssemblerBase
func (a *assemblerImpl) Assemble() ([]byte, error) {
	a.initializeNodesForEncoding()

	// Continue encoding until we are not forced to re-assemble which happens when
	// an short relative jump ends up the offset larger than 8-bit length.
	for {
		err := a.encode()
		if err != nil {
			return nil, err
		}

		if !a.forceReAssemble {
			break
		} else {
			// We reset the length of buffer but don't delete the underlying slice since
			// the binary size will roughly the same after reassemble.
			a.buf.Reset()
			// Reset the re-assemble flag in order to avoid the infinite loop!
			a.forceReAssemble = false
		}
	}

	code := a.buf.Bytes()
	for _, cb := range a.OnGenerateCallbacks {
		if err := cb(code); err != nil {
			return nil, err
		}
	}
	return code, nil
}

// initializeNodesForEncoding initializes nodeImpl.Flag and determine all the jumps
// are forward or backward jump.
func (a *assemblerImpl) initializeNodesForEncoding() {
	var count int
	for n := a.root; n != nil; n = n.next {
		count++
		n.flag |= nodeFlagInitializedForEncoding
		if target := n.jumpTarget; target != nil {
			if target.isInitializedForEncoding() {
				// This means the target exists behind.
				n.flag |= nodeFlagBackwardJump
			} else {
				// Otherwise, this is forward jump.
				// We start with assuming that the jump can be short (8-bit displacement).
				// If it doens't fit, we change this flag in resolveRelativeForwardJump.
				n.flag |= nodeFlagShortForwardJump
			}
		}
	}

	// Roughly allocate the buffer by assuming an instruction has 5-bytes length on average.
	a.buf.Grow(count * 5)
}

func (a *assemblerImpl) encode() (err error) {
	for n := a.root; n != nil; n = n.next {
		// If an instruction needs NOP padding, we do so before encoding it.
		// https://www.intel.com/content/dam/support/us/en/documents/processors/mitigations-jump-conditional-code-erratum.pdf
		if a.enablePadding {
			if err = a.maybeNOPPadding(n); err != nil {
				return
			}
		}

		// After the padding, we can finalize the offset of this instruction in the binary.
		n.offsetInBinary = (uint64(a.buf.Len()))

		if err := a.encodeNode(n); err != nil {
			return fmt.Errorf("%w: %v", err, n)
		}

		err = a.resolveForwardRelativeJumps(n)
		if err != nil {
			err = fmt.Errorf("invalid relative forward jumps: %w", err)
			break
		}
	}
	return
}

// maybeNOPpadding maybe appends NOP instructions before the node `n`.
// This is necessary to avoid Intel's jump erratum:
// https://www.intel.com/content/dam/support/us/en/documents/processors/mitigations-jump-conditional-code-erratum.pdf
func (a *assemblerImpl) maybeNOPPadding(n *nodeImpl) (err error) {
	var instructionLen int32

	// See in Section 2.1 in for when we have to pad NOP.
	// https://www.intel.com/content/dam/support/us/en/documents/processors/mitigations-jump-conditional-code-erratum.pdf
	switch n.instruction {
	case RET, JMP, JCC, JCS, JEQ, JGE, JGT, JHI, JLE, JLS, JLT, JMI, JNE, JPC, JPS:
		// In order to know the instruction length before writing into the binary,
		// we try encoding it with the temporary buffer.
		saved := a.buf
		a.buf = bytes.NewBuffer(nil)

		// Assign the temporary offset which may or may not be correct depending on the padding decision.
		n.offsetInBinary = uint64(saved.Len())

		// Encode the node and get the instruction length.
		if err = a.encodeNode(n); err != nil {
			return
		}
		instructionLen = int32(a.buf.Len())

		// Revert the temporary buffer.
		a.buf = saved
	case // The possible fused jump instructions if the next node is a conditional jump instruction.
		CMPL, CMPQ, TESTL, TESTQ, ADDL, ADDQ, SUBL, SUBQ, ANDL, ANDQ, INCQ, DECQ:
		instructionLen, err = a.fusedInstructionLength(n)
		if err != nil {
			return err
		}
	}

	if instructionLen == 0 {
		return
	}

	const boundaryInBytes int32 = 32
	const mask int32 = boundaryInBytes - 1

	var padNum int
	currentPos := int32(a.buf.Len())
	if used := currentPos & mask; used+instructionLen >= boundaryInBytes {
		padNum = int(boundaryInBytes - used)
	}

	a.padNOP(padNum)
	return
}

// fusedInstructionLength returns the length of "macro fused instruction" if the
// instruction sequence starting from `n` can be fused by processor. Otherwise,
// returns zero.
func (a *assemblerImpl) fusedInstructionLength(n *nodeImpl) (ret int32, err error) {
	// Find the next non-NOP instruction.
	next := n.next
	for ; next != nil && next.instruction == NOP; next = next.next {
	}

	if next == nil {
		return
	}

	inst, jmpInst := n.instruction, next.instruction

	if !(jmpInst == JCC || jmpInst == JCS || jmpInst == JEQ || jmpInst == JGE || jmpInst == JGT ||
		jmpInst == JHI || jmpInst == JLE || jmpInst == JLS || jmpInst == JLT || jmpInst == JMI ||
		jmpInst == JNE || jmpInst == JPC || jmpInst == JPS) {
		// If the next instruction is not jump kind, the instruction will not be fused.
		return
	}

	// How to determine whether or not the instruction can be fused is described in
	// Section 3.4.2.2 of "Intel Optimization Manual":
	// https://www.intel.com/content/dam/doc/manual/64-ia-32-architectures-optimization-manual.pdf
	isTest := inst == TESTL || inst == TESTQ
	isCmp := inst == CMPQ || inst == CMPL
	isTestCmp := isTest || isCmp
	if isTestCmp && ((n.types.src == operandTypeMemory && n.types.dst == operandTypeConst) ||
		(n.types.src == operandTypeConst && n.types.dst == operandTypeMemory)) {
		// The manual says: "CMP and TEST can not be fused when comparing MEM-IMM".
		return
	}

	// Implement the descision according to the table 3-1 in the manual.
	isAnd := inst == ANDL || inst == ANDQ
	if !isTest && !isAnd {
		if jmpInst == JMI || jmpInst == JPL || jmpInst == JPS || jmpInst == JPC {
			// These jumps are only fused for TEST or AND.
			return
		}
		isAdd := inst == ADDL || inst == ADDQ
		isSub := inst == SUBL || inst == SUBQ
		if !isCmp && !isAdd && !isSub {
			if jmpInst == JCS || jmpInst == JCC || jmpInst == JHI || jmpInst == JLS {
				// Thses jumpst are only fused for TEST, AND, CMP, ADD, or SUB.
				return
			}
		}
	}

	// Now the instruction is ensured to be fused by the processor.
	// In order to know the fused instruction length before writing into the binary,
	// we try encoding it with the temporary buffer.
	saved := a.buf
	savedLen := uint64(saved.Len())
	a.buf = bytes.NewBuffer(nil)

	for _, fused := range []*nodeImpl{n, next} {
		// Assign the temporary offset which may or may not be correct depending on the padding decision.
		fused.offsetInBinary = savedLen + uint64(a.buf.Len())

		// Encode the node into the temporary buffer.
		err = a.encodeNode(fused)
		if err != nil {
			return
		}
	}

	ret = int32(a.buf.Len())

	// Revert the temporary buffer.
	a.buf = saved
	return
}

// nopOpcodes is the multi byte NOP instructions table derived from section 5.8 "Code Padding with Operand-Size Override and Multibyte NOP"
// in "AMD Software Optimization Guide for AMD Family 15h Processors" https://www.amd.com/system/files/TechDocs/47414_15h_sw_opt_guide.pdf
//
// Note: We use up to 9 bytes NOP variant to line our implementation with Go's assembler.
// TODO: After golang-asm removal, add 9, 10 and 11 bytes variants.
var nopOpcodes = [][9]byte{
	{0x90},
	{0x66, 0x90},
	{0x0f, 0x1f, 0x00},
	{0x0f, 0x1f, 0x40, 0x00},
	{0x0f, 0x1f, 0x44, 0x00, 0x00},
	{0x66, 0x0f, 0x1f, 0x44, 0x00, 0x00},
	{0x0f, 0x1f, 0x80, 0x00, 0x00, 0x00, 0x00},
	{0x0f, 0x1f, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00},
	{0x66, 0x0f, 0x1f, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00},
}

func (a *assemblerImpl) padNOP(num int) {
	for num > 0 {
		singleNopNum := num
		if singleNopNum > len(nopOpcodes) {
			singleNopNum = len(nopOpcodes)
		}
		a.buf.Write(nopOpcodes[singleNopNum-1][:singleNopNum])
		num -= singleNopNum
	}
}

// CompileStandAlone implements asm.AssemblerBase.CompileStandAlone
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
	n := a.newNode(LEAQ, operandTypesMemoryToRegister)
	n.dstReg = destinationRegister
	n.readInstructionAddressBeforeTargetInstruction = beforeAcquisitionTargetInstruction
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

func errorEncodingUnsupported(n *nodeImpl) error {
	return fmt.Errorf("%s is unsupported for %s type", instructionName(n.instruction), n.types)
}

func (a *assemblerImpl) encodeNoneToNone(n *nodeImpl) (err error) {
	switch n.instruction {
	case CDQ:
		// https://www.felixcloutier.com/x86/cwd:cdq:cqo
		err = a.buf.WriteByte(0x99)
	case CQO:
		// https://www.felixcloutier.com/x86/cwd:cdq:cqo
		_, err = a.buf.Write([]byte{rexPrefixW, 0x99})
	case NOP:
		// Simply optimize out the NOP instructions.
	case RET:
		// https://www.felixcloutier.com/x86/ret
		err = a.buf.WriteByte(0xc3)
	default:
		err = errorEncodingUnsupported(n)
	}
	return
}

func (a *assemblerImpl) encodeNoneToRegister(n *nodeImpl) (err error) {
	regBits, prefix, err := register3bits(n.dstReg, registerSpecifierPositionModRMFieldRM)
	if err != nil {
		return err
	}

	// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
	modRM := 0b11_000_000 | // Specifying that opeand is register.
		regBits
	if n.instruction == JMP {
		// JMP's Opcode is defined as "FF /4" meaning that we have to have "4"
		// in 4-6th bits in the ModRM byte. https://www.felixcloutier.com/x86/jmp
		modRM |= 0b00_100_000
	} else {
		if REG_SP <= n.dstReg && n.dstReg <= REG_DI {
			// If the destination is one byte length register, we need to have the default prefix.
			// https: //wiki.osdev.org/X86-64_Instruction_Encoding#Registers
			prefix |= rexPrefixDefault
		}
	}

	if prefix != rexPrefixNone {
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#Encoding
		if err = a.buf.WriteByte(prefix); err != nil {
			return
		}
	}

	switch n.instruction {
	case JMP:
		// https://www.felixcloutier.com/x86/jmp
		_, err = a.buf.Write([]byte{0xff, modRM})
	case SETCC:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x93, modRM})
	case SETCS:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x92, modRM})
	case SETEQ:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x94, modRM})
	case SETGE:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9d, modRM})
	case SETGT:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9f, modRM})
	case SETHI:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x97, modRM})
	case SETLE:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9e, modRM})
	case SETLS:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x96, modRM})
	case SETLT:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9c, modRM})
	case SETNE:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x95, modRM})
	case SETPC:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9b, modRM})
	case SETPS:
		// https://www.felixcloutier.com/x86/setcc
		_, err = a.buf.Write([]byte{0x0f, 0x9a, modRM})
	default:
		err = errorEncodingUnsupported(n)
	}
	return
}

func (a *assemblerImpl) encodeNoneToMemory(n *nodeImpl) (err error) {
	rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
	if err != nil {
		return err
	}

	var opcode byte
	switch n.instruction {
	case INCQ:
		// https://www.felixcloutier.com/x86/inc
		rexPrefix |= rexPrefixW
		opcode = 0xff
	case DECQ:
		// https://www.felixcloutier.com/x86/dec
		rexPrefix |= rexPrefixW
		modRM |= 0b00_001_000 // DEC needs "/1" extension in ModRM.
		opcode = 0xff
	case JMP:
		// https://www.felixcloutier.com/x86/jmp
		modRM |= 0b00_100_000 // JMP needs "/4" extension in ModRM.
		opcode = 0xff
	default:
		return errorEncodingUnsupported(n)
	}

	if rexPrefix != rexPrefixNone {
		a.buf.WriteByte(rexPrefix)
	}

	a.buf.Write([]byte{opcode, modRM})

	if sbi != nil {
		a.buf.WriteByte(*sbi)
	}

	if displacementWidth != 0 {
		a.writeConst(n.dstConst, displacementWidth)
	}
	return
}

type relativeJumpOpcode struct{ short, long []byte }

func (o relativeJumpOpcode) instructionLen(short bool) int64 {
	if short {
		return int64(len(o.short)) + 1 // 1 byte = 8 bit offset
	} else {
		return int64(len(o.long)) + 4 // 4 byte = 32 bit offset
	}
}

var relativeJumpOpcodes = map[asm.Instruction]relativeJumpOpcode{
	// https://www.felixcloutier.com/x86/jcc
	JCC: {short: []byte{0x73}, long: []byte{0x0f, 0x83}},
	JCS: {short: []byte{0x72}, long: []byte{0x0f, 0x82}},
	JEQ: {short: []byte{0x74}, long: []byte{0x0f, 0x84}},
	JGE: {short: []byte{0x7d}, long: []byte{0x0f, 0x8d}},
	JGT: {short: []byte{0x7f}, long: []byte{0x0f, 0x8f}},
	JHI: {short: []byte{0x77}, long: []byte{0x0f, 0x87}},
	JLE: {short: []byte{0x7e}, long: []byte{0x0f, 0x8e}},
	JLS: {short: []byte{0x76}, long: []byte{0x0f, 0x86}},
	JLT: {short: []byte{0x7c}, long: []byte{0x0f, 0x8c}},
	JMI: {short: []byte{0x78}, long: []byte{0x0f, 0x88}},
	JNE: {short: []byte{0x75}, long: []byte{0x0f, 0x85}},
	JPC: {short: []byte{0x7b}, long: []byte{0x0f, 0x8b}},
	JPS: {short: []byte{0x7a}, long: []byte{0x0f, 0x8a}},
	// https://www.felixcloutier.com/x86/jmp
	JMP: {short: []byte{0xeb}, long: []byte{0xe9}},
}

func (a *assemblerImpl) resolveForwardRelativeJumps(target *nodeImpl) (err error) {
	offsetInBinary := int64(target.OffsetInBinary())
	for origin := range target.jumpOrigins {
		shortJump := origin.isForwardShortJump()
		op := relativeJumpOpcodes[origin.instruction]
		instructionLen := op.instructionLen(shortJump)

		// Calculate the offset from the EIP (at the time of executing this jump instruction)
		// to the target instruction. This value is always >= 0 as here we only handle forward jumps.
		offset := offsetInBinary - (int64(origin.OffsetInBinary()) + instructionLen)
		if shortJump {
			if offset > math.MaxInt8 {
				// This forces reassemble in the outer loop inside of assemblerImpl.Assemble().
				a.forceReAssemble = true
				// From the next reAssemble phases, this forward jump will be encoded long jump and
				// allocate 32-bit offset bytes by default. This means that this `origin` node
				// will always enter the "long jump offset encoding" block below
				origin.flag ^= nodeFlagShortForwardJump
			} else {
				a.buf.Bytes()[origin.OffsetInBinary()+uint64(instructionLen)-1] = byte(offset)
			}
		} else { // long jump offset encoding.
			if offset > math.MaxInt32 {
				return fmt.Errorf("too large jump offset %d for encoding %s", offset, instructionName(origin.instruction))
			}
			binary.LittleEndian.PutUint32(a.buf.Bytes()[origin.OffsetInBinary()+uint64(instructionLen)-4:], uint32(offset))
		}
	}
	return nil
}

func (a *assemblerImpl) encodeRelativeJump(n *nodeImpl) (err error) {
	if n.jumpTarget == nil {
		err = fmt.Errorf("jump traget must not be nil for relative %s", instructionName(n.instruction))
		return
	}

	op, ok := relativeJumpOpcodes[n.instruction]
	if !ok {
		return errorEncodingUnsupported(n)
	}

	var isShortJump bool
	// offsetOfEIP means the offset of EIP register at the time of executing this jump instruction.
	// Relative jump instructions can be encoded with the signed 8-bit or 32-bit integer offsets from the EIP.
	var offsetOfEIP int64 = 0 // We set zero and resolve later once the target instruction is encoded for forward jumps
	if n.isBackwardJump() {
		// If this is the backward jump, we can calculate the exact offset now.
		offsetOfJumpInstruction := int64(n.jumpTarget.OffsetInBinary()) - int64(n.OffsetInBinary())
		isShortJump = offsetOfJumpInstruction-2 >= math.MinInt8
		offsetOfEIP = offsetOfJumpInstruction - op.instructionLen(isShortJump)
	} else {
		// For forward jumps, we resolve the offset when we encode the target node. See assemblerImpl.resolveForwardRelativeJumps.
		n.jumpTarget.jumpOrigins[n] = struct{}{}
		isShortJump = n.isForwardShortJump()
	}

	if offsetOfEIP < math.MinInt32 { // offsetOfEIP is always <= 0 as we don't calculate it for forward jump here.
		return fmt.Errorf("too large jump offset %d for encoding %s", offsetOfEIP, instructionName(n.instruction))
	}

	if isShortJump {
		a.buf.Write(op.short)
		a.writeConst(offsetOfEIP, 8)
	} else {
		a.buf.Write(op.long)
		a.writeConst(offsetOfEIP, 32)
	}
	return
}

func (a *assemblerImpl) encodeRegisterToNone(n *nodeImpl) (err error) {
	regBits, prefix, err := register3bits(n.srcReg, registerSpecifierPositionModRMFieldRM)
	if err != nil {
		return err
	}

	// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
	modRM := 0b11_000_000 | // Specifying that opeand is register.
		regBits

	var opcode byte
	switch n.instruction {
	case DIVL:
		// https://www.felixcloutier.com/x86/div
		modRM |= 0b00_110_000
		opcode = 0xf7
	case DIVQ:
		// https://www.felixcloutier.com/x86/div
		prefix |= rexPrefixW
		modRM |= 0b00_110_000
		opcode = 0xf7
	case IDIVL:
		// https://www.felixcloutier.com/x86/idiv
		modRM |= 0b00_111_000
		opcode = 0xf7
	case IDIVQ:
		// https://www.felixcloutier.com/x86/idiv
		prefix |= rexPrefixW
		modRM |= 0b00_111_000
		opcode = 0xf7
	case MULL:
		// https://www.felixcloutier.com/x86/mul
		modRM |= 0b00_100_000
		opcode = 0xf7
	case MULQ:
		// https://www.felixcloutier.com/x86/mul
		prefix |= rexPrefixW
		modRM |= 0b00_100_000
		opcode = 0xf7
	default:
		err = errorEncodingUnsupported(n)
	}

	if prefix != rexPrefixNone {
		a.buf.WriteByte(prefix)
	}

	a.buf.Write([]byte{opcode, modRM})
	return
}

var registerToRegisterOpcode = map[asm.Instruction]struct {
	opcode                           []byte
	rPrefix                          rexPrefix
	mandatoryPrefix                  byte
	srcOnModRMReg                    bool
	isSrc8bit                        bool
	needMode                         bool
	requireSrcFloat, requireDstFloat bool
}{
	// https://www.felixcloutier.com/x86/add
	ADDL: {opcode: []byte{0x1}, srcOnModRMReg: true},
	ADDQ: {opcode: []byte{0x1}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/and
	ANDL: {opcode: []byte{0x21}, srcOnModRMReg: true},
	ANDQ: {opcode: []byte{0x21}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/cmp
	CMPL: {opcode: []byte{0x39}},
	CMPQ: {opcode: []byte{0x39}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/cmovcc
	CMOVQCS: {opcode: []byte{0x0f, 0x42}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/addsd
	ADDSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x58}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/addss
	ADDSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x58}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/addpd
	ANDPD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x54}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/addps
	ANDPS: {opcode: []byte{0x0f, 0x54}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/bsr
	BSRL: {opcode: []byte{0xf, 0xbd}},
	BSRQ: {opcode: []byte{0xf, 0xbd}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/comisd
	COMISD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x2f}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/comiss
	COMISS: {opcode: []byte{0x0f, 0x2f}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtsd2ss
	CVTSD2SS: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x5a}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtsi2sd
	CVTSL2SD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x2a}, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtsi2sd
	CVTSQ2SD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x2a}, rPrefix: rexPrefixW, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtsi2ss
	CVTSL2SS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x2a}, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtsi2ss
	CVTSQ2SS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x2a}, rPrefix: rexPrefixW, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvtss2sd
	CVTSS2SD: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x5a}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/cvttsd2si
	CVTTSD2SL: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x2c}, requireSrcFloat: true},
	CVTTSD2SQ: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x2c}, rPrefix: rexPrefixW, requireSrcFloat: true},
	// https://www.felixcloutier.com/x86/cvttss2si
	CVTTSS2SL: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x2c}, requireSrcFloat: true},
	CVTTSS2SQ: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x2c}, rPrefix: rexPrefixW, requireSrcFloat: true},
	// https://www.felixcloutier.com/x86/divsd
	DIVSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x5e}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/divss
	DIVSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x5e}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/lzcnt
	LZCNTL: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xbd}},
	LZCNTQ: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xbd}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/maxsd
	MAXSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x5f}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/maxss
	MAXSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x5f}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/minsd
	MINSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x5d}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/minss
	MINSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x5d}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/movsx:movsxd
	MOVBLSX: {opcode: []byte{0x0f, 0xbe}, isSrc8bit: true},
	// https://www.felixcloutier.com/x86/movzx
	MOVBLZX: {opcode: []byte{0x0f, 0xb6}, isSrc8bit: true},
	// https://www.felixcloutier.com/x86/movsx:movsxd
	MOVBQSX: {opcode: []byte{0x0f, 0xbe}, rPrefix: rexPrefixW, isSrc8bit: true},
	// https://www.felixcloutier.com/x86/movsx:movsxd
	MOVLQSX: {opcode: []byte{0x63}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/movsx:movsxd
	MOVWQSX: {opcode: []byte{0x0f, 0xbf}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/movsx:movsxd
	MOVWLSX: {opcode: []byte{0x0f, 0xbf}},
	// https://www.felixcloutier.com/x86/mulss
	MULSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x59}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/mulsd
	MULSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x59}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/or
	ORL: {opcode: []byte{0x09}, srcOnModRMReg: true},
	ORQ: {opcode: []byte{0x09}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/orpd
	ORPD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x56}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/orps
	ORPS: {opcode: []byte{0x0f, 0x56}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/popcnt
	POPCNTL: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xb8}},
	POPCNTQ: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xb8}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/roundss
	ROUNDSS: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x3a, 0x0a}, needMode: true, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/roundsd
	ROUNDSD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x3a, 0x0b}, needMode: true, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/sqrtss
	SQRTSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x51}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/sqrtsd
	SQRTSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x51}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/sub
	SUBL: {opcode: []byte{0x29}, srcOnModRMReg: true},
	SUBQ: {opcode: []byte{0x29}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/subss
	SUBSS: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0x5c}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/subsd
	SUBSD: {mandatoryPrefix: 0xf2, opcode: []byte{0x0f, 0x5c}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/test
	TESTL: {opcode: []byte{0x85}, srcOnModRMReg: true},
	TESTQ: {opcode: []byte{0x85}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/tzcnt
	TZCNTL: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xbc}},
	TZCNTQ: {mandatoryPrefix: 0xf3, opcode: []byte{0x0f, 0xbc}, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/ucomisd
	UCOMISD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x2e}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/ucomiss
	UCOMISS: {opcode: []byte{0x0f, 0x2e}, requireSrcFloat: true, requireDstFloat: true},
	// https://www.felixcloutier.com/x86/xor
	XORL: {opcode: []byte{0x31}, srcOnModRMReg: true},
	XORQ: {opcode: []byte{0x31}, rPrefix: rexPrefixW, srcOnModRMReg: true},
	// https://www.felixcloutier.com/x86/xorpd
	XORPD: {mandatoryPrefix: 0x66, opcode: []byte{0x0f, 0x57}, requireSrcFloat: true, requireDstFloat: true},
	XORPS: {opcode: []byte{0x0f, 0x57}, requireSrcFloat: true, requireDstFloat: true},
}

var registerToRegisterShiftOpcode = map[asm.Instruction]struct {
	opcode         []byte
	rPrefix        rexPrefix
	modRMExtension byte
}{
	// https://www.felixcloutier.com/x86/rcl:rcr:rol:ror
	ROLL: {opcode: []byte{0xd3}},
	ROLQ: {opcode: []byte{0xd3}, rPrefix: rexPrefixW},
	RORL: {opcode: []byte{0xd3}, modRMExtension: 0b00_001_000},
	RORQ: {opcode: []byte{0xd3}, modRMExtension: 0b00_001_000, rPrefix: rexPrefixW},
	// https://www.felixcloutier.com/x86/sal:sar:shl:shr
	SARL: {opcode: []byte{0xd3}, modRMExtension: 0b00_111_000},
	SARQ: {opcode: []byte{0xd3}, modRMExtension: 0b00_111_000, rPrefix: rexPrefixW},
	SHLL: {opcode: []byte{0xd3}, modRMExtension: 0b00_100_000},
	SHLQ: {opcode: []byte{0xd3}, modRMExtension: 0b00_100_000, rPrefix: rexPrefixW},
	SHRL: {opcode: []byte{0xd3}, modRMExtension: 0b00_101_000},
	SHRQ: {opcode: []byte{0xd3}, modRMExtension: 0b00_101_000, rPrefix: rexPrefixW},
}

type registerToRegisterMOVOpcode struct {
	opcode          []byte
	mandatoryPrefix byte
	srcOnModRMReg   bool
	rPrefix         rexPrefix
}

var registerToRegisterMOVOpcodes = map[asm.Instruction]struct {
	i2i, i2f, f2i, f2f registerToRegisterMOVOpcode
}{
	MOVL: {
		// https://www.felixcloutier.com/x86/mov
		i2i: registerToRegisterMOVOpcode{opcode: []byte{0x89}, srcOnModRMReg: true},
		// https://www.felixcloutier.com/x86/movd:movq
		i2f: registerToRegisterMOVOpcode{opcode: []byte{0x0f, 0x6e}, mandatoryPrefix: 0x66, srcOnModRMReg: false},
		f2i: registerToRegisterMOVOpcode{opcode: []byte{0x0f, 0x7e}, mandatoryPrefix: 0x66, srcOnModRMReg: true},
	},
	MOVQ: {
		// https://www.felixcloutier.com/x86/mov
		i2i: registerToRegisterMOVOpcode{opcode: []byte{0x89}, srcOnModRMReg: true, rPrefix: rexPrefixW},
		// https://www.felixcloutier.com/x86/movd:movq
		i2f: registerToRegisterMOVOpcode{opcode: []byte{0x0f, 0x6e}, mandatoryPrefix: 0x66, srcOnModRMReg: false, rPrefix: rexPrefixW},
		f2i: registerToRegisterMOVOpcode{opcode: []byte{0x0f, 0x7e}, mandatoryPrefix: 0x66, srcOnModRMReg: true, rPrefix: rexPrefixW},
		// https://www.felixcloutier.com/x86/movq
		f2f: registerToRegisterMOVOpcode{opcode: []byte{0x0f, 0x7e}, mandatoryPrefix: 0xf3},
	},
}

func (a *assemblerImpl) encodeRegisterToRegister(n *nodeImpl) (err error) {
	// Alias for readability
	inst := n.instruction

	if op, ok := registerToRegisterMOVOpcodes[inst]; ok {
		var opcode registerToRegisterMOVOpcode
		srcIsFloat, dstIsFloat := isFloatRegister(n.srcReg), isFloatRegister(n.dstReg)
		if srcIsFloat && dstIsFloat {
			if inst == MOVL {
				return errors.New("MOVL for float to float is undefined")
			}
			opcode = op.f2f
		} else if srcIsFloat && !dstIsFloat {
			opcode = op.f2i
		} else if !srcIsFloat && dstIsFloat {
			opcode = op.i2f
		} else {
			opcode = op.i2i
		}

		rexPrefix, modRM, err := n.getRegisterToRegisterModRM(opcode.srcOnModRMReg)
		if err != nil {
			return err
		}
		rexPrefix |= opcode.rPrefix

		if opcode.mandatoryPrefix != 0 {
			a.buf.WriteByte(opcode.mandatoryPrefix)
		}

		if rexPrefix != rexPrefixNone {
			a.buf.WriteByte(rexPrefix)
		}
		a.buf.Write(opcode.opcode)

		a.buf.WriteByte(modRM)
		return nil
	} else if op, ok := registerToRegisterOpcode[inst]; ok {
		srcIsFloat, dstIsFloat := isFloatRegister(n.srcReg), isFloatRegister(n.dstReg)
		if op.requireSrcFloat && !srcIsFloat {
			return fmt.Errorf("%s require float src register but got %s", instructionName(inst), registerName(n.srcReg))
		} else if op.requireDstFloat && !dstIsFloat {
			return fmt.Errorf("%s require float dst register but got %s", instructionName(inst), registerName(n.dstReg))
		} else if !op.requireSrcFloat && srcIsFloat {
			return fmt.Errorf("%s require integer src register but got %s", instructionName(inst), registerName(n.srcReg))
		} else if !op.requireDstFloat && dstIsFloat {
			return fmt.Errorf("%s require integer dst register but got %s", instructionName(inst), registerName(n.dstReg))
		}

		rexPrefix, modRM, err := n.getRegisterToRegisterModRM(op.srcOnModRMReg)
		if err != nil {
			return err
		}
		rexPrefix |= op.rPrefix

		if op.isSrc8bit && REG_SP <= n.srcReg && n.srcReg <= REG_DI {
			// If an operand register is 8-bit length of SP, BP, DI, or SI register, we need to have the default prefix.
			// https: //wiki.osdev.org/X86-64_Instruction_Encoding#Registers
			rexPrefix |= rexPrefixDefault
		}

		if op.mandatoryPrefix != 0 {
			a.buf.WriteByte(op.mandatoryPrefix)
		}

		if rexPrefix != rexPrefixNone {
			a.buf.WriteByte(rexPrefix)
		}
		a.buf.Write(op.opcode)

		a.buf.WriteByte(modRM)

		if op.needMode {
			a.writeConst(int64(n.mode), 8)
		}
		return nil
	} else if op, ok := registerToRegisterShiftOpcode[inst]; ok {
		if n.srcReg != REG_CX {
			return fmt.Errorf("shifting instruction %s require CX register as src but got %s", instructionName(inst), registerName(n.srcReg))
		} else if isFloatRegister(n.dstReg) {
			return fmt.Errorf("shifting instruction %s require integer register as dst but got %s", instructionName(inst), registerName(n.srcReg))
		}

		reg3bits, rexPrefix, err := register3bits(n.dstReg, registerSpecifierPositionModRMFieldRM)
		if err != nil {
			return err
		}

		rexPrefix |= op.rPrefix
		if rexPrefix != rexPrefixNone {
			a.buf.WriteByte(rexPrefix)
		}

		// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
		modRM := 0b11_000_000 |
			(op.modRMExtension) |
			reg3bits
		a.buf.Write(append(op.opcode, modRM))
		return nil
	} else {
		return errorEncodingUnsupported(n)
	}
}

func (a *assemblerImpl) encodeRegisterToMemory(n *nodeImpl) (err error) {
	rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
	if err != nil {
		return err
	}

	var opcode []byte
	var mandatoryPrefix byte
	var isShiftInstruction bool
	switch n.instruction {
	case CMPL:
		// https://www.felixcloutier.com/x86/cmp
		opcode = []byte{0x3b}
	case CMPQ:
		// https://www.felixcloutier.com/x86/cmp
		rexPrefix |= rexPrefixW
		opcode = []byte{0x3b}
	case MOVB:
		// https://www.felixcloutier.com/x86/mov
		opcode = []byte{0x88}
	case MOVL:
		if isFloatRegister(n.srcReg) {
			// https://www.felixcloutier.com/x86/movd:movq
			opcode = []byte{0x0f, 0x7e}
			mandatoryPrefix = 0x66
		} else {
			// https://www.felixcloutier.com/x86/mov
			opcode = []byte{0x89}
		}
	case MOVQ:
		if isFloatRegister(n.srcReg) {
			// https://www.felixcloutier.com/x86/movq
			opcode = []byte{0x0f, 0xd6}
			mandatoryPrefix = 0x66
		} else {
			// https://www.felixcloutier.com/x86/mov
			rexPrefix |= rexPrefixW
			opcode = []byte{0x89}
		}
	case MOVW:
		// https://www.felixcloutier.com/x86/mov
		// Note: Need 0x66 to indicate that the operand size is 16-bit.
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#Operand-size_and_address-size_override_prefix
		mandatoryPrefix = 0x66
		opcode = []byte{0x89}
	case SARL:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		modRM |= 0b00_111_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	case SARQ:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		rexPrefix |= rexPrefixW
		modRM |= 0b00_111_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	case SHLL:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		modRM |= 0b00_100_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	case SHLQ:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		rexPrefix |= rexPrefixW
		modRM |= 0b00_100_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	case SHRL:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		modRM |= 0b00_101_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	case SHRQ:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		rexPrefix |= rexPrefixW
		modRM |= 0b00_101_000
		opcode = []byte{0xd3}
		isShiftInstruction = true
	default:
		return errorEncodingUnsupported(n)
	}

	if !isShiftInstruction {
		srcReg3Bits, prefix, err := register3bits(n.srcReg, registerSpecifierPositionModRMFieldReg)
		if err != nil {
			return err
		}

		rexPrefix |= prefix
		modRM |= (srcReg3Bits << 3) // Place the source register on ModRM:reg
	} else {
		if n.srcReg != REG_CX {
			return fmt.Errorf("shifting instruction %s require CX register as src but got %s", instructionName(n.instruction), registerName(n.srcReg))
		}
	}

	if mandatoryPrefix != 0 {
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#Mandatory_prefix
		a.buf.WriteByte(mandatoryPrefix)
	}

	if rexPrefix != rexPrefixNone {
		a.buf.WriteByte(rexPrefix)
	}
	a.buf.Write(opcode)

	a.buf.WriteByte(modRM)

	if sbi != nil {
		a.buf.WriteByte(*sbi)
	}

	if displacementWidth != 0 {
		a.writeConst(n.dstConst, displacementWidth)
	}
	return
}

func (a *assemblerImpl) encodeRegisterToConst(n *nodeImpl) (err error) {
	regBits, prefix, err := register3bits(n.srcReg, registerSpecifierPositionModRMFieldRM)
	if err != nil {
		return err
	}

	switch n.instruction {
	case CMPL, CMPQ:
		if n.instruction == CMPQ {
			prefix |= rexPrefixW
		}
		if prefix != rexPrefixNone {
			a.buf.WriteByte(prefix)
		}
		is8bitConst := fitInSigned8bit(n.dstConst)
		// https://www.felixcloutier.com/x86/cmp
		if n.srcReg == REG_AX && !is8bitConst {
			a.buf.Write([]byte{0x3d})
		} else {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM := 0b11_000_000 | // Specifying that opeand is register.
				0b00_111_000 | // CMP with immediate needs "/7" extension.
				regBits
			if is8bitConst {
				a.buf.Write([]byte{0x83, modRM})
			} else {
				a.buf.Write([]byte{0x81, modRM})
			}
		}
	default:
		err = errorEncodingUnsupported(n)
	}

	if fitInSigned8bit(n.dstConst) {
		a.writeConst(n.dstConst, 8)
	} else {
		a.writeConst(n.dstConst, 32)
	}
	return
}

func (a *assemblerImpl) encodeReadInstructionAddress(n *nodeImpl) error {
	dstReg3Bits, rexPrefix, err := register3bits(n.dstReg, registerSpecifierPositionModRMFieldReg)
	if err != nil {
		return err
	}

	a.AddOnGenerateCallBack(func(code []byte) error {
		// Find the target instruction node.
		targetNode := n
		for ; targetNode != nil; targetNode = targetNode.next {
			if targetNode.instruction == n.readInstructionAddressBeforeTargetInstruction {
				targetNode = targetNode.next
				break
			}
		}

		if targetNode == nil {
			return errors.New("BUG: target instruction not found for read instruction address")
		}

		offset := targetNode.OffsetInBinary() - (n.OffsetInBinary() + 7 /* 7 = the length of the LEAQ instruction */)
		if offset >= math.MaxInt32 {
			return errors.New("BUG: too large offset for LEAQ instruction")
		}

		binary.LittleEndian.PutUint32(code[n.OffsetInBinary()+3:], uint32(int32(offset)))
		return nil
	})

	// https://www.felixcloutier.com/x86/lea
	opcode := byte(0x8d)
	rexPrefix |= rexPrefixW

	// https://wiki.osdev.org/X86-64_Instruction_Encoding#64-bit_addressing
	modRM := 0b00_000_101 | // Indicate "LEAQ [RIP + 32bit displacement], dstReg" encoding.
		(dstReg3Bits << 3) // Place the dstReg on ModRM:reg.

	a.buf.Write([]byte{rexPrefix, opcode, modRM})
	a.writeConst(int64(0), 32) // Preserve
	return nil
}

func (a *assemblerImpl) encodeMemoryToRegister(n *nodeImpl) (err error) {
	if n.instruction == LEAQ && n.readInstructionAddressBeforeTargetInstruction != NONE {
		return a.encodeReadInstructionAddress(n)
	}

	rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
	if err != nil {
		return err
	}

	dstReg3Bits, prefix, err := register3bits(n.dstReg, registerSpecifierPositionModRMFieldReg)
	if err != nil {
		return err
	}

	rexPrefix |= prefix
	modRM |= (dstReg3Bits << 3) // Place the destination register on ModRM:reg

	var mandatoryPrefix byte
	var opcode []byte
	switch n.instruction {
	case ADDL:
		// https://www.felixcloutier.com/x86/add
		opcode = []byte{0x03}
	case ADDQ:
		// https://www.felixcloutier.com/x86/add
		rexPrefix |= rexPrefixW
		opcode = []byte{0x03}
	case CMPL:
		// https://www.felixcloutier.com/x86/cmp
		opcode = []byte{0x39}
	case CMPQ:
		// https://www.felixcloutier.com/x86/cmp
		rexPrefix |= rexPrefixW
		opcode = []byte{0x39}
	case LEAQ:
		// https://www.felixcloutier.com/x86/lea
		rexPrefix |= rexPrefixW
		opcode = []byte{0x8d}
	case MOVBLSX:
		// https://www.felixcloutier.com/x86/movsx:movsxd
		opcode = []byte{0x0f, 0xbe}
	case MOVBLZX:
		// https://www.felixcloutier.com/x86/movzx
		opcode = []byte{0x0f, 0xb6}
	case MOVBQSX:
		// https://www.felixcloutier.com/x86/movsx:movsxd
		rexPrefix |= rexPrefixW
		opcode = []byte{0x0f, 0xbe}
	case MOVBQZX:
		// https://www.felixcloutier.com/x86/movzx
		rexPrefix |= rexPrefixW
		opcode = []byte{0x0f, 0xb6}
	case MOVLQSX:
		// https://www.felixcloutier.com/x86/movsx:movsxd
		rexPrefix |= rexPrefixW
		opcode = []byte{0x63}
	case MOVLQZX:
		// https://www.felixcloutier.com/x86/mov
		// Note: MOVLQZX means zero extending 32bit reg to 64-bit reg and
		// that is semantically equivalent to MOV 32bit to 32bit.
		opcode = []byte{0x8B}
	case MOVL:
		// https://www.felixcloutier.com/x86/mov
		// Note: MOVLQZX means zero extending 32bit reg to 64-bit reg and
		// that is semantically equivalent to MOV 32bit to 32bit.
		if isFloatRegister(n.dstReg) {
			// https://www.felixcloutier.com/x86/movd:movq
			opcode = []byte{0x0f, 0x6e}
			mandatoryPrefix = 0x66
		} else {
			// https://www.felixcloutier.com/x86/mov
			opcode = []byte{0x8B}
		}
	case MOVQ:
		if isFloatRegister(n.dstReg) {
			// https://www.felixcloutier.com/x86/movq
			opcode = []byte{0x0f, 0x7e}
			mandatoryPrefix = 0xf3
		} else {
			// https://www.felixcloutier.com/x86/mov
			rexPrefix |= rexPrefixW
			opcode = []byte{0x8B}
		}
	case MOVWLSX:
		// https://www.felixcloutier.com/x86/movsx:movsxd
		opcode = []byte{0x0f, 0xbf}
	case MOVWLZX:
		// https://www.felixcloutier.com/x86/movzx
		opcode = []byte{0x0f, 0xb7}
	case MOVWQSX:
		// https://www.felixcloutier.com/x86/movsx:movsxd
		rexPrefix |= rexPrefixW
		opcode = []byte{0x0f, 0xbf}
	case MOVWQZX:
		// https://www.felixcloutier.com/x86/movzx
		rexPrefix |= rexPrefixW
		opcode = []byte{0x0f, 0xb7}
	case SUBQ:
		// https://www.felixcloutier.com/x86/sub
		rexPrefix |= rexPrefixW
		opcode = []byte{0x2b}
	case SUBSD:
		// https://www.felixcloutier.com/x86/subsd
		opcode = []byte{0x0f, 0x5c}
		mandatoryPrefix = 0xf2
	case SUBSS:
		// https://www.felixcloutier.com/x86/subss
		opcode = []byte{0x0f, 0x5c}
		mandatoryPrefix = 0xf3
	case UCOMISD:
		// https://www.felixcloutier.com/x86/ucomisd
		opcode = []byte{0x0f, 0x2e}
		mandatoryPrefix = 0x66
	case UCOMISS:
		// https://www.felixcloutier.com/x86/ucomiss
		opcode = []byte{0x0f, 0x2e}
	default:
		return errorEncodingUnsupported(n)
	}

	if mandatoryPrefix != 0 {
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#Mandatory_prefix
		a.buf.WriteByte(mandatoryPrefix)
	}

	if rexPrefix != rexPrefixNone {
		a.buf.WriteByte(rexPrefix)
	}

	a.buf.Write(opcode)

	a.buf.WriteByte(modRM)

	if sbi != nil {
		a.buf.WriteByte(*sbi)
	}

	if displacementWidth != 0 {
		a.writeConst(n.srcConst, displacementWidth)
	}

	return
}

func (a *assemblerImpl) encodeConstToRegister(n *nodeImpl) (err error) {
	regBits, rexPrefix, err := register3bits(n.dstReg, registerSpecifierPositionModRMFieldRM)
	if err != nil {
		return err
	}

	isFloatReg := isFloatRegister(n.dstReg)
	switch n.instruction {
	case PSLLL, PSLLQ, PSRLL, PSRLQ:
		if !isFloatReg {
			return fmt.Errorf("%s needs float register but got %s", instructionName(n.instruction), registerName(n.dstReg))
		}
	default:
		if isFloatReg {
			return fmt.Errorf("%s needs int register but got %s", instructionName(n.instruction), registerName(n.dstReg))
		}
	}

	if n.instruction != MOVQ && !fitIn32bit(n.srcConst) {
		return fmt.Errorf("constant must fit in 32-bit integer for %s, but got %d", instructionName(n.instruction), n.srcConst)
	} else if (n.instruction == SHLQ || n.instruction == SHRQ) && (n.srcConst < 0 || n.srcConst > math.MaxUint8) {
		return fmt.Errorf("constant must fit in positive 8-bit integer for %s, but got %d", instructionName(n.instruction), n.srcConst)
	} else if (n.instruction == PSLLL ||
		n.instruction == PSLLQ ||
		n.instruction == PSRLL ||
		n.instruction == PSRLQ) && (n.srcConst < math.MinInt8 || n.srcConst > math.MaxInt8) {
		return fmt.Errorf("constant must fit in signed 8-bit integer for %s, but got %d", instructionName(n.instruction), n.srcConst)
	}

	isSigned8bitConst := fitInSigned8bit(n.srcConst)
	switch inst := n.instruction; inst {
	case ADDQ:
		// https://www.felixcloutier.com/x86/add
		rexPrefix |= rexPrefixW
		if n.dstReg == REG_AX && !isSigned8bitConst {
			a.buf.Write([]byte{rexPrefix, 0x05})
		} else {
			modRM := 0b11_000_000 | // Specifying that opeand is register.
				regBits
			if isSigned8bitConst {
				a.buf.Write([]byte{rexPrefix, 0x83, modRM})
			} else {
				a.buf.Write([]byte{rexPrefix, 0x81, modRM})
			}
		}
		if isSigned8bitConst {
			a.writeConst(n.srcConst, 8)
		} else {
			a.writeConst(n.srcConst, 32)
		}
	case ANDQ:
		// https://www.felixcloutier.com/x86/and
		rexPrefix |= rexPrefixW
		if n.dstReg == REG_AX && !isSigned8bitConst {
			a.buf.Write([]byte{rexPrefix, 0x25})
		} else {
			modRM := 0b11_000_000 | // Specifying that opeand is register.
				0b00_100_000 | // AND with immediate needs "/4" extension.
				regBits
			if isSigned8bitConst {
				a.buf.Write([]byte{rexPrefix, 0x83, modRM})
			} else {
				a.buf.Write([]byte{rexPrefix, 0x81, modRM})
			}
		}
		if fitInSigned8bit(n.srcConst) {
			a.writeConst(n.srcConst, 8)
		} else {
			a.writeConst(n.srcConst, 32)
		}
	case MOVL:
		// https://www.felixcloutier.com/x86/mov
		if rexPrefix != rexPrefixNone {
			a.buf.WriteByte(rexPrefix)
		}
		a.buf.Write([]byte{0xb8 | regBits})
		a.writeConst(n.srcConst, 32)
	case MOVQ:
		// https://www.felixcloutier.com/x86/mov
		if fitIn32bit(n.srcConst) {
			if n.srcConst > math.MaxInt32 {
				if rexPrefix != rexPrefixNone {
					a.buf.WriteByte(rexPrefix)
				}
				a.buf.Write([]byte{0xb8 | regBits})
			} else {
				rexPrefix |= rexPrefixW
				modRM := 0b11_000_000 | // Specifying that opeand is register.
					regBits
				a.buf.Write([]byte{rexPrefix, 0xc7, modRM})
			}
			a.writeConst(n.srcConst, 32)
		} else {
			rexPrefix |= rexPrefixW
			a.buf.Write([]byte{rexPrefix, 0xb8 | regBits})
			a.writeConst(n.srcConst, 64)
		}
	case SHLQ:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		rexPrefix |= rexPrefixW
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_100_000 | // SHL with immediate needs "/4" extension.
			regBits
		if n.srcConst == 1 {
			a.buf.Write([]byte{rexPrefix, 0xd1, modRM})
		} else {
			a.buf.Write([]byte{rexPrefix, 0xc1, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case SHRQ:
		// https://www.felixcloutier.com/x86/sal:sar:shl:shr
		rexPrefix |= rexPrefixW
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_101_000 | // SHR with immediate needs "/5" extension.
			regBits
		if n.srcConst == 1 {
			a.buf.Write([]byte{rexPrefix, 0xd1, modRM})
		} else {
			a.buf.Write([]byte{rexPrefix, 0xc1, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case PSLLL:
		// https://www.felixcloutier.com/x86/psllw:pslld:psllq
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_110_000 | // PSLL with immediate needs "/6" extension.
			regBits
		if rexPrefix != rexPrefixNone {
			a.buf.Write([]byte{0x66, rexPrefix, 0x0f, 0x72, modRM})
			a.writeConst(n.srcConst, 8)
		} else {
			a.buf.Write([]byte{0x66, 0x0f, 0x72, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case PSLLQ:
		// https://www.felixcloutier.com/x86/psllw:pslld:psllq
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_110_000 | // PSLL with immediate needs "/6" extension.
			regBits
		if rexPrefix != rexPrefixNone {
			a.buf.Write([]byte{0x66, rexPrefix, 0x0f, 0x73, modRM})
			a.writeConst(n.srcConst, 8)
		} else {
			a.buf.Write([]byte{0x66, 0x0f, 0x73, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case PSRLL:
		// https://www.felixcloutier.com/x86/psrlw:psrld:psrlq
		// https://www.felixcloutier.com/x86/psllw:pslld:psllq
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_010_000 | // PSRL with immediate needs "/2" extension.
			regBits
		if rexPrefix != rexPrefixNone {
			a.buf.Write([]byte{0x66, rexPrefix, 0x0f, 0x72, modRM})
			a.writeConst(n.srcConst, 8)
		} else {
			a.buf.Write([]byte{0x66, 0x0f, 0x72, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case PSRLQ:
		// https://www.felixcloutier.com/x86/psrlw:psrld:psrlq
		modRM := 0b11_000_000 | // Specifying that opeand is register.
			0b00_010_000 | // PSRL with immediate needs "/2" extension.
			regBits
		if rexPrefix != rexPrefixNone {
			a.buf.Write([]byte{0x66, rexPrefix, 0x0f, 0x73, modRM})
			a.writeConst(n.srcConst, 8)
		} else {
			a.buf.Write([]byte{0x66, 0x0f, 0x73, modRM})
			a.writeConst(n.srcConst, 8)
		}
	case XORL, XORQ:
		// https://www.felixcloutier.com/x86/xor
		if inst == XORQ {
			rexPrefix |= rexPrefixW
		}
		if rexPrefix != rexPrefixNone {
			a.buf.WriteByte(rexPrefix)
		}
		if n.dstReg == REG_AX && !isSigned8bitConst {
			a.buf.Write([]byte{0x35})
		} else {
			modRM := 0b11_000_000 | // Specifying that opeand is register.
				0b00_110_000 | // XOR with immediate needs "/6" extension.
				regBits
			if isSigned8bitConst {
				a.buf.Write([]byte{0x83, modRM})
			} else {
				a.buf.Write([]byte{0x81, modRM})
			}
		}
		if fitInSigned8bit(n.srcConst) {
			a.writeConst(n.srcConst, 8)
		} else {
			a.writeConst(n.srcConst, 32)
		}
	default:
		err = errorEncodingUnsupported(n)
	}
	return
}

func (a *assemblerImpl) encodeMemoryToConst(n *nodeImpl) (err error) {
	if !fitIn32bit(n.dstConst) {
		return fmt.Errorf("too large target const %d for %s", n.dstConst, instructionName(n.instruction))
	}

	rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
	if err != nil {
		return err
	}

	// Alias for readability.
	c := n.dstConst

	var opcode, constWidth byte
	switch n.instruction {
	case CMPL:
		// https://www.felixcloutier.com/x86/cmp
		if fitInSigned8bit(c) {
			opcode = 0x83
			constWidth = 8
		} else {
			opcode = 0x81
			constWidth = 32
		}
		modRM |= 0b00_111_000
	default:
		return errorEncodingUnsupported(n)
	}

	if rexPrefix != rexPrefixNone {
		a.buf.WriteByte(rexPrefix)
	}

	a.buf.Write([]byte{opcode, modRM})

	if sbi != nil {
		a.buf.WriteByte(*sbi)
	}

	if displacementWidth != 0 {
		a.writeConst(n.srcConst, displacementWidth)
	}

	a.writeConst(c, constWidth)
	return
}

func (a *assemblerImpl) encodeConstToMemory(n *nodeImpl) (err error) {
	rexPrefix, modRM, sbi, displacementWidth, err := n.getMemoryLocation()
	if err != nil {
		return err
	}

	// Alias for readability.
	inst := n.instruction
	c := n.srcConst

	if inst == MOVB && !fitInSigned8bit(c) {
		return fmt.Errorf("too large load target const %d for MOVB", c)
	} else if !fitIn32bit(c) {
		return fmt.Errorf("too large load target const %d for %s", c, instructionName(n.instruction))
	}

	var constWidth, opcode byte
	switch inst {
	case MOVB:
		opcode = 0xc6
		constWidth = 8
	case MOVL:
		opcode = 0xc7
		constWidth = 32
	case MOVQ:
		rexPrefix |= rexPrefixW
		opcode = 0xc7
		constWidth = 32
	default:
		return errorEncodingUnsupported(n)
	}

	if rexPrefix != rexPrefixNone {
		a.buf.WriteByte(rexPrefix)
	}

	a.buf.Write([]byte{opcode, modRM})

	if sbi != nil {
		a.buf.WriteByte(*sbi)
	}

	if displacementWidth != 0 {
		a.writeConst(n.dstConst, displacementWidth)
	}

	a.writeConst(c, constWidth)
	return
}

func (a *assemblerImpl) writeConst(v int64, length byte) {
	switch length {
	case 8:
		a.buf.WriteByte(byte(int8(v)))
	case 32:
		// TODO: any way to directly put little endian bytes into bytes.Buffer?
		offsetBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetBytes, uint32(int32(v)))
		a.buf.Write(offsetBytes)
	case 64:
		// TODO: any way to directly put little endian bytes into bytes.Buffer?
		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, uint64(v))
		a.buf.Write(offsetBytes)
	default:
		panic("BUG: length must be one of 8, 32 or 64")
	}
}

func (n *nodeImpl) getMemoryLocation() (p rexPrefix, modRM byte, sbi *byte, displacementWidth byte, err error) {
	var baseReg, indexReg asm.Register
	var offset asm.ConstantValue
	var scale byte
	if n.types.dst == operandTypeMemory {
		baseReg, offset, indexReg, scale = n.dstReg, n.dstConst, n.dstMemIndex, n.dstMemScale
	} else if n.types.src == operandTypeMemory {
		baseReg, offset, indexReg, scale = n.srcReg, n.srcConst, n.srcMemIndex, n.srcMemScale
	} else {
		err = fmt.Errorf("memory location is not supported for %s", n.types)
		return
	}

	if !fitIn32bit(offset) {
		err = errors.New("offset does not fit in 32-bit integer")
		return
	}

	if baseReg == asm.NilRegister && indexReg != asm.NilRegister {
		// [(index*scale) + displacement] addressing is possible, but we haven't used it for now.
		err = errors.New("addressing without base register but with index is not implemented")
	} else if baseReg == asm.NilRegister {
		modRM = 0b00_000_100 // Indicate that the memory location is specified by SIB.
		sbiValue := byte(0b00_100_101)
		sbi = &sbiValue
		displacementWidth = 32
	} else if indexReg == asm.NilRegister {
		modRM, p, err = register3bits(baseReg, registerSpecifierPositionModRMFieldRM)
		if err != nil {
			return
		}

		// Create ModR/M byte so that this instrction takes [R/M + displacement] operand if displacement !=0
		// and otherwise [R/M].
		withoutDisplacement := offset == 0 &&
			// If the target register is R13 or BP, we have to keep [R/M + displacement] even if the value
			// is zero since it's not [R/M] operand is not defined for these two registers.
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#32.2F64-bit_addressing
			baseReg != REG_R13 && baseReg != REG_BP
		if withoutDisplacement {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b00_000_000 // Specifying that operand is memory without displacement
			displacementWidth = 0
		} else if fitInSigned8bit(offset) {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b01_000_000 // Specifying that operand is memory + 8bit displacement.
			displacementWidth = 8
		} else {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b10_000_000 // Specifying that operand is memory + 32bit displacement.
			displacementWidth = 32
		}

		// For SP and R12 register, we have [SIB + displacement] if the const is non-zero, otherwise [SIP].
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#32.2F64-bit_addressing
		//
		// Thefore we emit the SIB byte before the const so that [SIB + displacement] ends up [register + displacement].
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#32.2F64-bit_addressing_2
		if baseReg == REG_SP || baseReg == REG_R12 {
			sbiValue := byte(0b00_100_100)
			sbi = &sbiValue
		}
	} else {
		if indexReg == REG_SP {
			err = errors.New("SP cannot be used for SIB index")
			return
		}

		modRM = 0b00_000_100 // Indicate that the memory location is specified by SIB.

		withoutDisplacement := offset == 0 &&
			// For R13 and BP, base registers cannot be encoded "without displacement" mod (i.e. 0b00 mod).
			baseReg != REG_R13 && baseReg != REG_BP
		if withoutDisplacement {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b00_000_000 // Specifying that operand is SIB without displacement
			displacementWidth = 0
		} else if fitInSigned8bit(offset) {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b01_000_000 // Specifying that operand is SIB + 8bit displacement.
			displacementWidth = 8
		} else {
			// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
			modRM |= 0b10_000_000 // Specifying that operand is SIB + 32bit displacement.
			displacementWidth = 32
		}

		var baseRegBits byte
		baseRegBits, p, err = register3bits(baseReg, registerSpecifierPositionModRMFieldRM)
		if err != nil {
			return
		}

		var indexRegBits byte
		var indexRegPrefix rexPrefix
		indexRegBits, indexRegPrefix, err = register3bits(indexReg, registerSpecifierPositionSIBIndex)
		if err != nil {
			return
		}
		p |= indexRegPrefix

		sbiValue := baseRegBits | (indexRegBits << 3)
		switch scale {
		case 1:
			sbiValue |= 0b00_000_000
		case 2:
			sbiValue |= 0b01_000_000
		case 4:
			sbiValue |= 0b10_000_000
		case 8:
			sbiValue |= 0b11_000_000
		default:
			err = fmt.Errorf("scale in SIB must be one of 1, 2, 4, 8 but got %d", scale)
			return
		}

		sbi = &sbiValue
	}
	return
}

// TODO: srcOnModRMReg can be deleted after golang-asm removal. This is necessary to match our implementation
// with golang-asm, but in practice, there are equivalent opcodes to always have src on ModRM:reg without ambiguity.
func (n *nodeImpl) getRegisterToRegisterModRM(srcOnModRMReg bool) (rexPrefix, modRM byte, err error) {
	var reg3bits, rm3bits byte
	if srcOnModRMReg {
		reg3bits, rexPrefix, err = register3bits(n.srcReg,
			// Indicate that srcReg will be specified by ModRM:reg.
			registerSpecifierPositionModRMFieldReg)
		if err != nil {
			return
		}

		var dstRexPrefix byte
		rm3bits, dstRexPrefix, err = register3bits(n.dstReg,
			// Indicate that dstReg will be specified by ModRM:r/m.
			registerSpecifierPositionModRMFieldRM)
		if err != nil {
			return
		}
		rexPrefix |= dstRexPrefix
	} else {
		rm3bits, rexPrefix, err = register3bits(n.srcReg,
			// Indicate that srcReg will be specified by ModRM:r/m.
			registerSpecifierPositionModRMFieldRM)
		if err != nil {
			return
		}

		var dstRexPrefix byte
		reg3bits, dstRexPrefix, err = register3bits(n.dstReg,
			// Indicate that dstReg will be specified by ModRM:reg.
			registerSpecifierPositionModRMFieldReg)
		if err != nil {
			return
		}
		rexPrefix |= dstRexPrefix
	}

	// https://wiki.osdev.org/X86-64_Instruction_Encoding#ModR.2FM
	modRM = 0b11_000_000 | // Specifying that dst opeand is register.
		(reg3bits << 3) |
		rm3bits

	return
}

// rexPrefix represents REX prefix https://wiki.osdev.org/X86-64_Instruction_Encoding#REX_prefix
type rexPrefix = byte

// REX prefixes are independent of each other and can be combined with OR.
const (
	rexPrefixNone    rexPrefix = 0x0000_0000 // Indicates that the instruction doesn't need rexPrefix.
	rexPrefixDefault rexPrefix = 0b0100_0000
	rexPrefixW       rexPrefix = 0b0000_1000 | rexPrefixDefault
	rexPrefixR       rexPrefix = 0b0000_0100 | rexPrefixDefault
	rexPrefixX       rexPrefix = 0b0000_0010 | rexPrefixDefault
	rexPrefixB       rexPrefix = 0b0000_0001 | rexPrefixDefault
)

// registerSpecifierPosition represents the position in the instruction bytes where an operand register is placed.
type registerSpecifierPosition byte

const (
	registerSpecifierPositionModRMFieldReg registerSpecifierPosition = iota
	registerSpecifierPositionModRMFieldRM
	registerSpecifierPositionSIBIndex
)

func register3bits(reg asm.Register, registerSpecifierPosition registerSpecifierPosition) (bits byte, prefix rexPrefix, err error) {
	prefix = rexPrefixNone
	if REG_R8 <= reg && reg <= REG_R15 || REG_X8 <= reg && reg <= REG_X15 {
		// https://wiki.osdev.org/X86-64_Instruction_Encoding#REX_prefix
		switch registerSpecifierPosition {
		case registerSpecifierPositionModRMFieldReg:
			prefix = rexPrefixR
		case registerSpecifierPositionModRMFieldRM:
			prefix = rexPrefixB
		case registerSpecifierPositionSIBIndex:
			prefix = rexPrefixX
		}
	}

	// https://wiki.osdev.org/X86-64_Instruction_Encoding#Registers
	switch reg {
	case REG_AX, REG_R8, REG_X0, REG_X8:
		bits = 0b000
	case REG_CX, REG_R9, REG_X1, REG_X9:
		bits = 0b001
	case REG_DX, REG_R10, REG_X2, REG_X10:
		bits = 0b010
	case REG_BX, REG_R11, REG_X3, REG_X11:
		bits = 0b011
	case REG_SP, REG_R12, REG_X4, REG_X12:
		bits = 0b100
	case REG_BP, REG_R13, REG_X5, REG_X13:
		bits = 0b101
	case REG_SI, REG_R14, REG_X6, REG_X14:
		bits = 0b110
	case REG_DI, REG_R15, REG_X7, REG_X15:
		bits = 0b111
	default:
		err = fmt.Errorf("invalid register [%s]", registerName(reg))
	}
	return
}

func fitIn32bit(v int64) bool {
	return math.MinInt32 <= v && v <= math.MaxUint32
}

func fitInSigned8bit(v int64) bool {
	return math.MinInt8 <= v && v <= math.MaxInt8
}

func isFloatRegister(r asm.Register) bool {
	return REG_X0 <= r && r <= REG_X15
}
